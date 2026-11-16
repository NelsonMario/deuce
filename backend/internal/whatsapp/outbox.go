package whatsapp

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"

	"deuce/backend/internal/match"
	"deuce/backend/internal/session"
)

type OutboxWorker struct {
	client   Client
	repo     Repository
	logger   *slog.Logger
	sessions session.Repository
	matches  match.ReadRepository
}

func NewOutboxWorker(client Client, repo Repository, sessions session.Repository, matches match.ReadRepository, logger *slog.Logger) *OutboxWorker {
	return &OutboxWorker{client: client, repo: repo, sessions: sessions, matches: matches, logger: logger}
}

func (w *OutboxWorker) Run(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.process(ctx)
		}
	}
}

func (w *OutboxWorker) process(ctx context.Context) {
	if !w.client.IsConnected() {
		return
	}
	items, err := w.repo.ClaimPending(ctx, 10)
	if err != nil {
		w.logger.Warn("whatsapp_outbox_claim_failed", "error", err)
		return
	}
	for _, item := range items {
		if w.isStale(ctx, item) {
			if err := w.repo.Cancel(ctx, item.ID); err != nil {
				w.logger.Warn("whatsapp_outbox_cancel_failed", "item_id", item.ID, "error", err)
			}
			continue
		}
		jid, err := types.ParseJID(item.GroupJID)
		if err == nil {
			err = w.client.SendMessage(ctx, jid, formatAnnouncement(item.Payload))
		}
		if err != nil {
			delay := time.Duration(item.Attempts*5) * time.Second
			_ = w.repo.Retry(ctx, item.ID, err.Error(), time.Now().Add(delay))
			w.logger.Warn("whatsapp_announcement_failed", "match_id", item.MatchID, "error", err)
			continue
		}
		if err := w.repo.MarkSent(ctx, item.ID); err != nil {
			w.logger.Warn("whatsapp_outbox_mark_sent_failed", "error", err)
		}
	}
}

// isStale prevents a queued announcement from being sent after its match was
// cancelled/finished or its session was ended. A message that was already
// accepted by WhatsApp cannot be recalled, so the outbox protects only work
// that is still pending.
func (w *OutboxWorker) isStale(ctx context.Context, item OutboxItem) bool {
	sess, err := w.sessions.GetByID(ctx, item.SessionID)
	if err != nil || sess.Status != session.StatusActive {
		return true
	}
	m, err := w.matches.GetByID(ctx, item.MatchID)
	return err != nil || m.Status == match.StatusFinished
}

func formatAnnouncement(a Announcement) string {
	return fmt.Sprintf("🏸 *Match Assignment*\n\n*%s*\n%s / %s\nvs\n%s / %s\n\nFormat: %s", a.CourtName, nameAt(a.TeamA, 0), nameAt(a.TeamA, 1), nameAt(a.TeamB, 0), nameAt(a.TeamB, 1), a.Format)
}

func nameAt(names []string, index int) string {
	if index < len(names) && strings.TrimSpace(names[index]) != "" {
		return names[index]
	}
	return "Player"
}
