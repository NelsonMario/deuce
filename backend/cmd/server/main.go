// Command server runs the badminton club backend HTTP API.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"deuce/backend/internal/auth"
	"deuce/backend/internal/club"
	"deuce/backend/internal/config"
	"deuce/backend/internal/database"
	"deuce/backend/internal/device"
	"deuce/backend/internal/httpapi"
	"deuce/backend/internal/httpapi/handler"
	"deuce/backend/internal/httpapi/middleware"
	"deuce/backend/internal/lock"
	"deuce/backend/internal/match"
	"deuce/backend/internal/player"
	"deuce/backend/internal/ratelimit"
	"deuce/backend/internal/session"
)

// matchGenLockTTL bounds the double-tap guard on match generation (see
// internal/lock and match.Service.GenerateAutomatic/ConfirmManual).
const matchGenLockTTL = 5 * time.Second

// autoFillLockTTL bounds the auto-fill poller's per-session coordination
// lock (see internal/lock and match.AutoFillPoller).
const autoFillLockTTL = 10 * time.Second

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	logger := newLogger(cfg.LogLevel)
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := database.NewPool(ctx, cfg.DatabaseURL())
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	hasher := auth.NewHasher(cfg.PlayerTokenSecret)
	redisClient, closeRedis := newRedisClient(ctx, cfg.RedisURL(), logger)
	defer closeRedis()

	// The v2 device-identity index, the match-generation double-tap lock,
	// the shared rate limiter, and the auto-fill poller's per-session
	// coordination lock all share this one Redis connection; each falls
	// back independently to a Noop implementation (never blocking real
	// work) when Redis is unset or unreachable.
	var deviceLinker device.Linker = device.NoopLinker{}
	var matchLocker lock.Locker = lock.NoopLocker{}
	var rateLimiter ratelimit.Limiter = ratelimit.NoopLimiter{}
	var autoFillLocker lock.Locker = lock.NoopLocker{}
	if redisClient != nil {
		deviceLinker = device.NewRedisLinker(redisClient, logger)
		matchLocker = lock.NewRedisLocker(redisClient, matchGenLockTTL, logger)
		rateLimiter = ratelimit.NewRedisLimiter(redisClient, middleware.DefaultRateLimitMax, middleware.DefaultRateLimitWindow, logger)
		autoFillLocker = lock.NewRedisLocker(redisClient, autoFillLockTTL, logger)
		logger.Info("device_identity_enabled")
		logger.Info("match_generation_lock_enabled")
		logger.Info("rate_limiter_shared_enabled")
		logger.Info("auto_fill_lock_enabled")
	} else {
		logger.Info("device_identity_disabled")
		logger.Info("match_generation_lock_disabled")
		logger.Info("rate_limiter_shared_disabled", "reason", "Redis unset/unreachable, requests are not rate-limited")
		logger.Info("auto_fill_lock_disabled")
	}

	playerRepo := player.NewRepository(pool)
	clubRepo := club.NewRepository(pool)
	sessionRepo := session.NewRepository(pool)
	matchReadRepo := match.NewReadRepository(pool)

	playerService := player.NewService(playerRepo)
	clubService := club.NewService(clubRepo, playerRepo, hasher, deviceLinker, cfg.JoinCodeLength, logger)
	sessionService := session.NewService(sessionRepo, clubRepo, playerRepo, hasher, deviceLinker, logger)
	matchService := match.NewService(pool, matchReadRepo, matchLocker, logger)

	handlers := handler.New(clubService, playerService, sessionService, matchService, logger)

	app := httpapi.NewApp(httpapi.Deps{
		Handlers:    handlers,
		Hasher:      hasher,
		Players:     playerRepo,
		Logger:      logger,
		CORSOrigins: cfg.CORSOrigins,
		RateLimiter: rateLimiter,
	})

	go func() {
		addr := ":" + cfg.Port
		logger.Info("server_starting", "addr", addr, "env", cfg.AppEnv)
		if err := app.Listen(addr); err != nil {
			logger.Error("server error", "error", err)
			stop()
		}
	}()

	// AutoFillPoller implements fully-automatic mode: it keeps generating
	// matches for every eligible session's empty courts with no host
	// trigger. It shares sessionRepo and matchService with the HTTP layer
	// and runs for as long as ctx is live, so it stops on the same
	// SIGINT/SIGTERM signal that starts the HTTP server's graceful shutdown
	// below.
	autoFillPoller := match.NewAutoFillPoller(sessionRepo, matchService, autoFillLocker, logger)
	var autoFillDone sync.WaitGroup
	autoFillDone.Add(1)
	go func() {
		defer autoFillDone.Done()
		autoFillPoller.Run(ctx)
	}()

	<-ctx.Done()
	logger.Info("shutting_down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := app.ShutdownWithContext(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
	}
	autoFillDone.Wait()
}

// newRedisClient sets up the single optional Redis connection shared by the
// v2 device-identity index (internal/device) and the match-generation
// double-tap lock (internal/lock). If redisURL is empty or the instance
// can't be reached, it logs a warning and returns a nil client; callers
// fall back to their respective Noop implementations so the app still runs
// with plain v1 behavior (no device recognition, no double-tap guard)
// rather than failing to start.
func newRedisClient(ctx context.Context, redisURL string, logger *slog.Logger) (*redis.Client, func()) {
	noop := func() {}
	if redisURL == "" {
		logger.Info("redis_client_disabled", "reason", "REDIS_HOST not set")
		return nil, noop
	}

	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		logger.Warn("redis_client_disabled", "reason", "invalid Redis config", "error", err)
		return nil, noop
	}

	client := redis.NewClient(opts)
	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		logger.Warn("redis_client_disabled", "reason", "redis unreachable", "error", err)
		_ = client.Close()
		return nil, noop
	}

	return client, func() { _ = client.Close() }
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = slog.LevelInfo
	}
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})
	return slog.New(handler)
}
