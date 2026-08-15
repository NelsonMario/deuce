package ratelimit

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// fakeCommander is an in-memory stand-in for the Redis Incr/Expire calls
// RedisLimiter depends on, so the counting/threshold logic can be tested
// without a real Redis instance.
type fakeCommander struct {
	counts       map[string]int64
	expireCalled map[string]time.Duration
	incrErr      error
	expireErr    error
}

func newFakeCommander() *fakeCommander {
	return &fakeCommander{
		counts:       make(map[string]int64),
		expireCalled: make(map[string]time.Duration),
	}
}

func (f *fakeCommander) Incr(ctx context.Context, key string) *redis.IntCmd {
	if f.incrErr != nil {
		return redis.NewIntResult(0, f.incrErr)
	}
	f.counts[key]++
	return redis.NewIntResult(f.counts[key], nil)
}

func (f *fakeCommander) Expire(ctx context.Context, key string, expiration time.Duration) *redis.BoolCmd {
	if f.expireErr != nil {
		return redis.NewBoolResult(false, f.expireErr)
	}
	f.expireCalled[key] = expiration
	return redis.NewBoolResult(true, nil)
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(discardWriter{}, nil))
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

func newTestLimiter(fc *fakeCommander, max int64, window time.Duration) *RedisLimiter {
	return &RedisLimiter{client: fc, max: max, window: window, logger: discardLogger()}
}

func TestRedisLimiter_AllowsUpToMax(t *testing.T) {
	fc := newFakeCommander()
	l := newTestLimiter(fc, 3, time.Minute)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if !l.Allow(ctx, "player:abc") {
			t.Fatalf("expected request %d to be allowed", i+1)
		}
	}
}

func TestRedisLimiter_RejectsOverMax(t *testing.T) {
	fc := newFakeCommander()
	l := newTestLimiter(fc, 3, time.Minute)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		l.Allow(ctx, "player:abc")
	}
	if l.Allow(ctx, "player:abc") {
		t.Fatalf("expected 4th request to be rejected once max is exceeded")
	}
}

func TestRedisLimiter_KeysAreIndependent(t *testing.T) {
	fc := newFakeCommander()
	l := newTestLimiter(fc, 1, time.Minute)
	ctx := context.Background()

	if !l.Allow(ctx, "player:a") {
		t.Fatalf("expected first request for player:a to be allowed")
	}
	if !l.Allow(ctx, "player:b") {
		t.Fatalf("expected first request for player:b to be allowed independently of player:a")
	}
	if l.Allow(ctx, "player:a") {
		t.Fatalf("expected second request for player:a to be rejected")
	}
}

func TestRedisLimiter_ArmsExpiryOnFirstHitOnly(t *testing.T) {
	fc := newFakeCommander()
	window := 30 * time.Second
	l := newTestLimiter(fc, 5, window)
	ctx := context.Background()

	l.Allow(ctx, "player:a")
	if got := fc.expireCalled["player:a"]; got != window {
		t.Fatalf("expected Expire to be called with window %v on first hit, got %v", window, got)
	}

	fc.expireCalled = make(map[string]time.Duration)
	l.Allow(ctx, "player:a")
	if _, called := fc.expireCalled["player:a"]; called {
		t.Fatalf("expected Expire not to be called again on subsequent hits within the window")
	}
}

func TestRedisLimiter_FailsOpenOnIncrError(t *testing.T) {
	fc := newFakeCommander()
	fc.incrErr = errors.New("redis unavailable")
	l := newTestLimiter(fc, 1, time.Minute)
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		if !l.Allow(ctx, "player:a") {
			t.Fatalf("expected Allow to fail open (return true) when Incr errors")
		}
	}
}

func TestRedisLimiter_FailsOpenOnExpireError(t *testing.T) {
	fc := newFakeCommander()
	fc.expireErr = errors.New("redis unavailable")
	l := newTestLimiter(fc, 1, time.Minute)
	ctx := context.Background()

	// Expire failing must not block the (still successfully counted) request.
	if !l.Allow(ctx, "player:a") {
		t.Fatalf("expected Allow to succeed even when the best-effort Expire call fails")
	}
}

func TestNoopLimiter_AlwaysAllows(t *testing.T) {
	var l Limiter = NoopLimiter{}
	ctx := context.Background()
	for i := 0; i < 100; i++ {
		if !l.Allow(ctx, "anything") {
			t.Fatalf("expected NoopLimiter to always allow")
		}
	}
}
