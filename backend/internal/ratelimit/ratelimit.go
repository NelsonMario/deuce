// Package ratelimit provides a shared, Redis-backed request budget so the
// limit is enforced across all replicas (unlike an in-memory, per-process
// counter) and can be keyed by whatever the caller chooses — e.g. bearer
// token instead of IP, so players sharing one venue's WiFi don't share one
// budget.
//
// This uses a simple fixed-window counter (INCR + EXPIRE on first increment
// within the window) rather than a sliding window: correctness and
// simplicity are favored over precision at window edges, which is an
// acceptable trade-off for a "protect against accidental hammering" budget.
//
// If Redis is unset, unreachable, or errors, Allow degrades to "always
// allow" (NoopLimiter / RedisLimiter-on-error) rather than blocking
// legitimate work — the same fail-open philosophy as internal/lock's
// NoopLocker and internal/device's NoopLinker.
package ratelimit

import (
	"context"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

const opTimeout = 750 * time.Millisecond

// Limiter reports whether a request identified by key is within budget.
// Implementations must never block legitimate callers on Redis being slow
// or down: Allow returns true on any error.
type Limiter interface {
	// Allow returns true if the request under key is within the configured
	// budget for the current window (and counts against it), or false if
	// the budget has been exceeded.
	Allow(ctx context.Context, key string) bool
}

// redisCommander is the subset of *redis.Client this package needs. It
// exists so tests can substitute a fake backing store without a real Redis
// instance; *redis.Client satisfies it.
type redisCommander interface {
	Incr(ctx context.Context, key string) *redis.IntCmd
	Expire(ctx context.Context, key string, expiration time.Duration) *redis.BoolCmd
}

// RedisLimiter is the real implementation, backed by a shared Redis client.
// Each key gets its own fixed window: the first request in a window sets
// the key's expiry to Window; subsequent requests in the same window just
// increment the counter.
type RedisLimiter struct {
	client redisCommander
	max    int64
	window time.Duration
	logger *slog.Logger
}

// NewRedisLimiter builds a RedisLimiter allowing up to max requests per
// window for any given key. Intended for the same shared *redis.Client used
// by device.RedisLinker and lock.RedisLocker, not a dedicated connection.
func NewRedisLimiter(client *redis.Client, max int64, window time.Duration, logger *slog.Logger) *RedisLimiter {
	return &RedisLimiter{client: client, max: max, window: window, logger: logger}
}

func (l *RedisLimiter) Allow(ctx context.Context, key string) bool {
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()

	count, err := l.client.Incr(ctx, key).Result()
	if err != nil {
		l.logger.Warn("ratelimit_check_failed", "key", key, "error", err)
		return true // degrade to "always allow" rather than blocking real work
	}
	if count == 1 {
		// First hit in this window: arm the expiry so the counter resets.
		// Best-effort — if this fails the key may live longer than Window
		// (worst case: briefly stricter, never a full outage), so it's not
		// treated as a reason to reject the request.
		if err := l.client.Expire(ctx, key, l.window).Err(); err != nil {
			l.logger.Warn("ratelimit_expire_failed", "key", key, "error", err)
		}
	}
	return count <= l.max
}

// NoopLimiter is used when REDIS_HOST isn't configured (or Redis is
// unreachable at startup): every request is allowed, so callers are never
// blocked by the guard this package provides — only the guard itself is
// disabled.
type NoopLimiter struct{}

func (NoopLimiter) Allow(context.Context, string) bool { return true }
