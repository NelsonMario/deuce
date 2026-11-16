package whatsapp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"deuce/backend/internal/match"
)

type Repository interface {
	ClaimPending(ctx context.Context, limit int) ([]OutboxItem, error)
	MarkSent(ctx context.Context, id uuid.UUID) error
	Retry(ctx context.Context, id uuid.UUID, reason string, next time.Time) error
	Cancel(ctx context.Context, id uuid.UUID) error
	PendingCount(ctx context.Context) (int64, error)
}

type pgRepository struct {
	pool     *pgxpool.Pool
	groupJID string
}

func NewRepository(pool *pgxpool.Pool, groupJID string) *pgRepository {
	return &pgRepository{pool: pool, groupJID: groupJID}
}

// Enqueue implements match.AnnouncementOutbox. The insert uses the match
// transaction, so a committed assignment always has a durable group message.
func (r *pgRepository) Enqueue(ctx context.Context, tx pgx.Tx, announcement match.Announcement) error {
	body, err := json.Marshal(Announcement{CourtName: announcement.CourtName, Format: announcement.Format, TeamA: announcement.TeamA[:], TeamB: announcement.TeamB[:]})
	if err != nil {
		return fmt.Errorf("marshal announcement: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO whatsapp_group_outbox (session_id, match_id, group_jid, payload)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (match_id) DO NOTHING`, announcement.SessionID, announcement.MatchID, r.groupJID, body)
	return err
}

func (r *pgRepository) ClaimPending(ctx context.Context, limit int) ([]OutboxItem, error) {
	rows, err := r.pool.Query(ctx, `
		WITH claimed AS (
			SELECT id FROM whatsapp_group_outbox
			WHERE (status = 'PENDING' AND next_attempt_at <= now())
			   OR (status = 'SENDING' AND updated_at < now() - interval '5 minutes')
			ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT $1
		)
		UPDATE whatsapp_group_outbox o SET status = 'SENDING', attempts = attempts + 1, updated_at = now()
		FROM claimed WHERE o.id = claimed.id
		RETURNING o.id, o.session_id, o.match_id, o.group_jid, o.payload, o.status, o.attempts, o.next_attempt_at, o.created_at`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []OutboxItem
	for rows.Next() {
		var item OutboxItem
		var body []byte
		if err := rows.Scan(&item.ID, &item.SessionID, &item.MatchID, &item.GroupJID, &body, &item.Status, &item.Attempts, &item.NextAttempt, &item.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(body, &item.Payload); err != nil {
			return nil, fmt.Errorf("decode announcement: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *pgRepository) MarkSent(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `UPDATE whatsapp_group_outbox SET status = 'SENT', sent_at = now(), updated_at = now() WHERE id = $1 AND status = 'SENDING'`, id)
	return err
}

func (r *pgRepository) Retry(ctx context.Context, id uuid.UUID, reason string, next time.Time) error {
	_, err := r.pool.Exec(ctx, `UPDATE whatsapp_group_outbox SET status = CASE WHEN attempts >= 5 THEN 'FAILED' ELSE 'PENDING' END, last_error = $2, next_attempt_at = $3, updated_at = now() WHERE id = $1 AND status = 'SENDING'`, id, reason, next)
	return err
}

func (r *pgRepository) Cancel(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `UPDATE whatsapp_group_outbox SET status = 'CANCELLED', updated_at = now() WHERE id = $1 AND status IN ('PENDING', 'SENDING')`, id)
	return err
}

func (r *pgRepository) PendingCount(ctx context.Context) (int64, error) {
	var count int64
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM whatsapp_group_outbox WHERE status IN ('PENDING', 'SENDING')`).Scan(&count)
	return count, err
}
