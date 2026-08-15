// Package lock provides a short-lived, best-effort distributed lock backed
// by Redis. It exists as a fast pre-check to reject an obviously duplicate
// concurrent request (e.g. a double-click or slow-network retry resubmit)
// before a slower, authoritative lock (e.g. a Postgres SELECT ... FOR
// UPDATE transaction) even begins — additive to that locking, never a
// replacement for it.
//
// This is deliberately not a general-purpose distributed lock: there is no
// ownership token or fencing, so it must not be used anywhere correctness
// depends on mutual exclusion actually holding under all failure modes.
// TTL-bounded expiry is the only backstop against a holder that never
// releases (e.g. a crashed process). If Redis is unset, unreachable, or
// errors, acquisition silently degrades to "always allow" (NoopLocker)
// rather than blocking legitimate work — the same fail-open philosophy as
// internal/device's NoopLinker.
package lock

import (
	"context"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

const opTimeout = 750 * time.Millisecond

// Locker attempts to acquire a short-lived exclusive lock for a key.
// Implementations must never block legitimate callers on Redis being slow
// or down: TryAcquire returns true on any error, and Release is always a
// best-effort no-op on error.
type Locker interface {
	// TryAcquire returns true if the lock was newly acquired (the caller
	// may proceed), or false if another holder currently has it (the
	// caller should fail fast rather than proceed).
	TryAcquire(ctx context.Context, key string) bool
	// Release drops the lock early so a legitimate retry after a finished
	// (successful or failed) attempt isn't stuck waiting out the full TTL.
	// Always best-effort; callers should not treat failures as fatal.
	Release(ctx context.Context, key string)
}

// RedisLocker is the real implementation, backed by a shared Redis client.
// Acquisition uses SETNX-with-expiry semantics (Redis `SET key val EX ttl
// NX`, exposed by go-redis as SetNX with an expiration) so the lock and its
// TTL are set atomically in one round trip.
type RedisLocker struct {
	client *redis.Client
	ttl    time.Duration
	logger *slog.Logger
}

// NewRedisLocker builds a RedisLocker whose locks expire after ttl if never
// released. Intended for the same shared *redis.Client used by
// device.RedisLinker, not a dedicated connection.
func NewRedisLocker(client *redis.Client, ttl time.Duration, logger *slog.Logger) *RedisLocker {
	return &RedisLocker{client: client, ttl: ttl, logger: logger}
}

func (l *RedisLocker) TryAcquire(ctx context.Context, key string) bool {
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()

	ok, err := l.client.SetNX(ctx, key, "1", l.ttl).Result()
	if err != nil {
		l.logger.Warn("lock_acquire_failed", "key", key, "error", err)
		return true // degrade to "always allow" rather than blocking real work
	}
	return ok
}

func (l *RedisLocker) Release(ctx context.Context, key string) {
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	if err := l.client.Del(ctx, key).Err(); err != nil {
		l.logger.Warn("lock_release_failed", "key", key, "error", err)
	}
}

// NoopLocker is used when REDIS_HOST isn't configured (or Redis is
// unreachable at startup): every acquisition succeeds immediately, so
// callers are never blocked by the guard this package provides — only the
// guard itself is disabled.
type NoopLocker struct{}

func (NoopLocker) TryAcquire(context.Context, string) bool { return true }
func (NoopLocker) Release(context.Context, string)         {}
