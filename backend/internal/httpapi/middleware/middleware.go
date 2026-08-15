// Package middleware provides Fiber middleware for request ID propagation,
// structured logging, panic recovery, rate limiting, and player-token
// authentication.
package middleware

import (
	"log/slog"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"

	"deuce/backend/internal/apperr"
	"deuce/backend/internal/auth"
	"deuce/backend/internal/player"
	"deuce/backend/internal/ratelimit"
)

// RequestID assigns/propagates an X-Request-Id header.
func RequestID() fiber.Handler {
	return requestid.New()
}

// Recover converts panics into 500 responses instead of crashing the server.
func Recover() fiber.Handler {
	return recover.New()
}

// CORS restricts cross-origin requests to the configured origins.
func CORS(origins []string) fiber.Handler {
	return cors.New(cors.Config{
		AllowOrigins: strings.Join(origins, ","),
		AllowHeaders: "Origin, Content-Type, Accept, Authorization",
		AllowMethods: "GET,POST,PATCH,PUT,DELETE,OPTIONS",
	})
}

// DefaultRateLimitMax and DefaultRateLimitWindow define the request budget
// RateLimit is intended to be configured with: 60 requests per minute per
// key, matching the original in-memory/per-IP limit this replaced. Exported
// so cmd/server/main.go can build the shared ratelimit.RedisLimiter with the
// same figures RateLimit's doc comment advertises.
const (
	DefaultRateLimitMax    int64 = 60
	DefaultRateLimitWindow       = time.Minute
)

// RateLimit applies a shared request budget, backed by limiter (typically
// ratelimit.RedisLimiter so the budget is enforced across all replicas
// instead of per-process), to protect matchmaking/mutating endpoints from
// accidental hammering by clients.
//
// Requests are keyed by bearer token (hashed the same way as for
// authentication — see auth.Hasher — so raw session tokens are never held
// in Redis) when one is present, so players sharing a venue's WiFi (and
// therefore an IP) don't share one budget. Requests with no bearer token
// (e.g. the auth-free club-create/join endpoints) fall back to being keyed
// by IP.
func RateLimit(limiter ratelimit.Limiter, hasher auth.Hasher) fiber.Handler {
	return func(c *fiber.Ctx) error {
		key := "ip:" + c.IP()
		if raw, ok := bearerToken(c); ok {
			key = "token:" + hasher.Hash(raw)
		}

		if !limiter.Allow(c.UserContext(), "ratelimit:"+key) {
			return apperr.RateLimited("too many requests, please slow down")
		}
		return c.Next()
	}
}

// bearerToken extracts the raw token from a "Authorization: Bearer <token>"
// header, if present and well-formed. Shared by RateLimit (keying) and
// RequireAuth (resolving the caller).
func bearerToken(c *fiber.Ctx) (string, bool) {
	header := c.Get(fiber.HeaderAuthorization)
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return "", false
	}
	raw := strings.TrimPrefix(header, prefix)
	if raw == "" {
		return "", false
	}
	return raw, true
}

// Logging emits one structured log line per request.
func Logging(logger *slog.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		err := c.Next()
		status := c.Response().StatusCode()

		attrs := []any{
			"request_id", c.GetRespHeader(fiber.HeaderXRequestID),
			"method", c.Method(),
			"path", c.Path(),
			"status", status,
			"duration_ms", time.Since(start).Milliseconds(),
		}
		if err != nil {
			attrs = append(attrs, "error", err.Error())
		}

		if status >= 500 {
			logger.Error("http_request", attrs...)
		} else {
			logger.Info("http_request", attrs...)
		}
		return err
	}
}

// RequireAuth extracts the "Authorization: Bearer <token>" header, resolves
// it to a player via the token hash (raw tokens are never stored — see
// internal/auth), and attaches the resulting auth.Principal to the request
// context for downstream handlers.
func RequireAuth(hasher auth.Hasher, players player.Repository) fiber.Handler {
	return func(c *fiber.Ctx) error {
		raw, ok := bearerToken(c)
		if !ok {
			return apperr.Unauthorized("missing bearer token")
		}

		p, err := players.GetByTokenHash(c.UserContext(), hasher.Hash(raw))
		if err != nil {
			return apperr.Unauthorized("invalid or expired token")
		}

		principal := auth.Principal{PlayerID: p.ID}
		c.SetUserContext(auth.WithPrincipal(c.UserContext(), principal))
		c.Locals("principal", principal)
		return c.Next()
	}
}
