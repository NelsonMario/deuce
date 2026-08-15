// Package device implements the v2 "lightweight identity layer": an
// optional Redis-backed index that lets a returning browser/device be
// recognized as the same player within a club, so their rating and match
// history carry over instead of starting fresh at every join.
//
// This is deliberately NOT an account system: there is no login, no
// password, no cross-device recovery. It only makes joins *idempotent* for
// the device that made them, scoped to a single club. Redis holds only the
// device_id -> player_id index (a few dozen bytes per entry); Postgres
// remains the source of truth for everything else. If Redis is unset,
// unreachable, or errors, linking silently degrades to v1 behavior (every
// join creates a brand-new player) rather than failing the request.
package device

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"deuce/backend/internal/player"
)

// linkTTL bounds how long an inactive device link is kept, so an abandoned
// club's entries eventually free up space on a small (e.g. free-tier) Redis
// instance. Refreshed on every successful lookup or link.
const linkTTL = 180 * 24 * time.Hour

const opTimeout = 750 * time.Millisecond

// Linker resolves and records which player a device is, within a club.
// Implementations must never block the caller on Redis being slow or down:
// Lookup returns ok=false and Link is a best-effort no-op on any error.
type Linker interface {
	Lookup(ctx context.Context, deviceID string, clubID uuid.UUID) (playerID uuid.UUID, ok bool)
	Link(ctx context.Context, deviceID string, clubID, playerID uuid.UUID)
}

// RedisLinker is the real implementation, backed by a Redis (e.g. Redis
// Cloud free-tier) instance.
type RedisLinker struct {
	client *redis.Client
	logger *slog.Logger
}

func NewRedisLinker(client *redis.Client, logger *slog.Logger) *RedisLinker {
	return &RedisLinker{client: client, logger: logger}
}

func linkKey(deviceID string, clubID uuid.UUID) string {
	return fmt.Sprintf("device:%s:club:%s", deviceID, clubID)
}

func (l *RedisLinker) Lookup(ctx context.Context, deviceID string, clubID uuid.UUID) (uuid.UUID, bool) {
	if deviceID == "" {
		return uuid.Nil, false
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()

	key := linkKey(deviceID, clubID)
	val, err := l.client.Get(ctx, key).Result()
	if err != nil {
		if err != redis.Nil {
			l.logger.Warn("device_lookup_failed", "error", err)
		}
		return uuid.Nil, false
	}
	playerID, err := uuid.Parse(val)
	if err != nil {
		l.logger.Warn("device_lookup_invalid_value", "error", err)
		return uuid.Nil, false
	}
	l.client.Expire(ctx, key, linkTTL) // refresh TTL on active use, best-effort
	return playerID, true
}

func (l *RedisLinker) Link(ctx context.Context, deviceID string, clubID, playerID uuid.UUID) {
	if deviceID == "" {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	if err := l.client.Set(ctx, linkKey(deviceID, clubID), playerID.String(), linkTTL).Err(); err != nil {
		l.logger.Warn("device_link_failed", "error", err)
	}
}

// NoopLinker is used when REDIS_HOST isn't configured: every join behaves
// exactly like v1 (always creates a fresh player identity).
type NoopLinker struct{}

func (NoopLinker) Lookup(context.Context, string, uuid.UUID) (uuid.UUID, bool) { return uuid.Nil, false }
func (NoopLinker) Link(context.Context, string, uuid.UUID, uuid.UUID)          {}

// Resolve either recognizes a returning device — reusing its player (rating
// and history carry over, its profile refreshed to the display name/gender
// just submitted) — or creates a brand-new player identity, linking the
// device to it for next time. Shared by club.Service and session.Service,
// which both need identical join-time identity resolution.
func Resolve(
	ctx context.Context,
	linker Linker,
	players player.Repository,
	deviceID string,
	clubID uuid.UUID,
	displayName string,
	gender player.Gender,
) (player.Player, bool, error) {
	if playerID, ok := linker.Lookup(ctx, deviceID, clubID); ok {
		if existing, err := players.GetByID(ctx, playerID); err == nil {
			updated, err := players.UpdateProfile(ctx, existing.ID, displayName, gender)
			if err != nil {
				return player.Player{}, false, fmt.Errorf("update returning player profile: %w", err)
			}
			return updated, false, nil
		}
		// Linked player vanished somehow — fall through and create a new one.
	}

	p, err := players.Create(ctx, displayName, gender)
	if err != nil {
		return player.Player{}, false, fmt.Errorf("create player: %w", err)
	}
	if _, err := players.CreateRating(ctx, p.ID, player.InitialRating); err != nil {
		return player.Player{}, false, fmt.Errorf("create player rating: %w", err)
	}
	linker.Link(ctx, deviceID, clubID, p.ID)
	return p, true, nil
}
