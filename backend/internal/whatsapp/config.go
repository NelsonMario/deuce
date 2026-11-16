package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Config is the bot's runtime setup, stored in the database and changed
// from WhatsApp with `setup` commands, so no .env edit or restart is needed.
type Config struct {
	ClubID *uuid.UUID
	// AdminGroup is where admins type commands; every member is an admin.
	AdminGroup string
	// AnnounceGroup only receives announcements (lineups, results).
	AnnounceGroup string
}

// ConfigStore reads and updates the bot's setup.
type ConfigStore interface {
	Get(ctx context.Context) (Config, error)
	SetAdminGroup(ctx context.Context, groupJID string, clubID uuid.UUID) error
	SetAnnounceGroup(ctx context.Context, groupJID string) error
}

// configCacheTTL keeps per-message reads cheap while letting a `setup`
// done through another replica take effect within seconds.
const configCacheTTL = 5 * time.Second

type pgConfigStore struct {
	pool *pgxpool.Pool

	mu       sync.Mutex
	cached   Config
	cachedAt time.Time
}

func NewConfigStore(pool *pgxpool.Pool) *pgConfigStore {
	return &pgConfigStore{pool: pool}
}

func (s *pgConfigStore) Get(ctx context.Context) (Config, error) {
	s.mu.Lock()
	if !s.cachedAt.IsZero() && time.Since(s.cachedAt) < configCacheTTL {
		cfg := s.cached
		s.mu.Unlock()
		return cfg, nil
	}
	s.mu.Unlock()

	var cfg Config
	var clubID *uuid.UUID
	var admin, announce *string
	err := s.pool.QueryRow(ctx,
		`SELECT club_id, admin_group_jid, announce_group_jid FROM whatsapp_bot_config WHERE id = 1`,
	).Scan(&clubID, &admin, &announce)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Config{}, fmt.Errorf("load whatsapp bot config: %w", err)
	}
	cfg.ClubID = clubID
	if admin != nil {
		cfg.AdminGroup = *admin
	}
	if announce != nil {
		cfg.AnnounceGroup = *announce
	}

	s.mu.Lock()
	s.cached, s.cachedAt = cfg, time.Now()
	s.mu.Unlock()
	return cfg, nil
}

func (s *pgConfigStore) SetAdminGroup(ctx context.Context, groupJID string, clubID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO whatsapp_bot_config (id, club_id, admin_group_jid) VALUES (1, $1, $2)
		ON CONFLICT (id) DO UPDATE SET club_id = EXCLUDED.club_id,
			admin_group_jid = EXCLUDED.admin_group_jid, updated_at = now()`, clubID, groupJID)
	s.invalidate()
	return err
}

func (s *pgConfigStore) SetAnnounceGroup(ctx context.Context, groupJID string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO whatsapp_bot_config (id, announce_group_jid) VALUES (1, $1)
		ON CONFLICT (id) DO UPDATE SET announce_group_jid = EXCLUDED.announce_group_jid, updated_at = now()`, groupJID)
	s.invalidate()
	return err
}

// SeedLegacy copies the pre-`setup` .env values (WHATSAPP_CLUB_ID and
// WHATSAPP_GROUP_JID) into the database the first time the bot starts with
// them, so existing deployments keep working. It never overwrites a setup
// already stored.
func (s *pgConfigStore) SeedLegacy(ctx context.Context, clubID *uuid.UUID, groupJID string) error {
	var group *string
	if groupJID != "" {
		group = &groupJID
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO whatsapp_bot_config (id, club_id, announce_group_jid) VALUES (1, $1, $2)
		ON CONFLICT (id) DO NOTHING`, clubID, group)
	s.invalidate()
	return err
}

func (s *pgConfigStore) invalidate() {
	s.mu.Lock()
	s.cachedAt = time.Time{}
	s.mu.Unlock()
}
