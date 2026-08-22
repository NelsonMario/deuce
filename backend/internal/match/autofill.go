package match

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"deuce/backend/internal/apperr"
	"deuce/backend/internal/lock"
	"deuce/backend/internal/session"
)

// AutoFillFormat is the match format the auto-fill poller uses when it
// generates a match with no host input. It matches the frontend's default
// selection for the host-triggered "Generate match" flow (see
// frontend/src/routes/session/[sessionId]/+page.svelte, autoFormat).
const AutoFillFormat = MixedDoubles

// autoFillInterval is how often the poller re-scans for sessions that need
// filling. A few seconds keeps courts busy without hammering Postgres with
// an empty-result query once every session finishes.
const autoFillInterval = 4 * time.Second

// autoFillLockTTL bounds how long a session's auto-fill coordination lock
// can survive if Release is never reached (e.g. the process crashes
// mid-tick). It only needs to outlast one tick's worth of work for a single
// session, so it is generous relative to autoFillInterval without risking a
// stuck session if a replica dies.
const autoFillLockTTL = 10 * time.Second

// autoFillLockKey scopes the coordination lock to one session: it guards
// "should this replica be the one scanning this session's courts right
// now". Correctness of the match itself (a session_player or court never
// being double-assigned) still comes entirely from GenerateAutomatic's own
// Postgres row locking (SELECT ... FOR UPDATE) — this lock is only a
// best-effort optimization to avoid two replicas doing redundant work (and
// bouncing off each other's INSUFFICIENT_PLAYERS/CONFLICT responses) on the
// same session in the same tick.
func autoFillLockKey(sessionID string) string {
	return fmt.Sprintf("autofill:session:%s", sessionID)
}

// minPlayersPerMatch mirrors the doubles format matchmaking.GenerateMatch
// requires: two players per team, two teams.
const minPlayersPerMatch = 4

// AutoFillPoller periodically scans for ACTIVE, AUTOMATIC-assignment
// sessions with auto_fill_enabled set and generates matches for any of
// their empty courts that have enough WAITING players — the "fully
// automatic" mode described in spec: no host has to tap "Generate match"
// for every court.
//
// It is additive on top of match.Service.GenerateAutomatic, which it calls
// unmodified: all of the transactional correctness (row locking, the
// double-tap guard keyed per court) still lives there. This poller only
// decides *when* and *for which court* to call it.
type AutoFillPoller struct {
	sessions session.Repository
	matches  *Service
	locker   lock.Locker
	logger   *slog.Logger
	interval time.Duration
}

// NewAutoFillPoller builds a poller sharing the same session repository,
// match service, and Redis-backed locker already wired up by cmd/server.
func NewAutoFillPoller(sessions session.Repository, matches *Service, locker lock.Locker, logger *slog.Logger) *AutoFillPoller {
	return &AutoFillPoller{sessions: sessions, matches: matches, locker: locker, logger: logger, interval: autoFillInterval}
}

// Run ticks every autoFillInterval until ctx is cancelled, running one scan
// per tick. It never returns an error: per-session and per-court failures
// are logged and skipped so one bad session can't stall the others or crash
// the goroutine, matching the fail-open philosophy of internal/lock and
// internal/device.
func (p *AutoFillPoller) Run(ctx context.Context) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	p.logger.Info("autofill_poller_started", "interval", p.interval)
	for {
		select {
		case <-ctx.Done():
			p.logger.Info("autofill_poller_stopped")
			return
		case <-ticker.C:
			p.tick(ctx)
		}
	}
}

// tick runs one scan across every eligible session. Errors listing sessions
// are logged; a lookup failure on one tick just means the next tick tries
// again.
func (p *AutoFillPoller) tick(ctx context.Context) {
	sessions, err := p.sessions.ListActiveAutoFillSessions(ctx)
	if err != nil {
		p.logger.Warn("autofill_list_sessions_failed", "error", err)
		return
	}
	for _, sess := range sessions {
		p.processSession(ctx, sess)
	}
}

// processSession fills as many of one session's empty courts as its
// current WAITING headcount allows. The session-scoped lock is a fast,
// best-effort guard against two replicas working the same session in the
// same tick — real correctness still comes from GenerateAutomatic's own
// Postgres row locking, so a failure to acquire this lock just means "skip
// this session this tick", never a correctness problem.
func (p *AutoFillPoller) processSession(ctx context.Context, sess session.Session) {
	lockKey := autoFillLockKey(sess.ID.String())
	if !p.locker.TryAcquire(ctx, lockKey) {
		return
	}
	defer p.locker.Release(ctx, lockKey)

	waiting, err := p.sessions.CountWaitingPlayers(ctx, sess.ID)
	if err != nil {
		p.logger.Warn("autofill_count_waiting_failed", "session_id", sess.ID, "error", err)
		return
	}
	if waiting < minPlayersPerMatch {
		return
	}

	courts, err := p.sessions.ListAvailableCourts(ctx, sess.ID)
	if err != nil {
		p.logger.Warn("autofill_list_courts_failed", "session_id", sess.ID, "error", err)
		return
	}

	for _, court := range courts {
		if waiting < minPlayersPerMatch {
			break
		}
		m, err := p.matches.GenerateAutomatic(ctx, GenerateInput{
			SessionID: sess.ID,
			CourtID:   court.ID,
			Format:    AutoFillFormat,
		})
		if err != nil {
			// InsufficientPlayers (another court in this same loop, or a
			// concurrent host/replica action, just took the remaining WAITING
			// players) and Conflict (the court was concurrently claimed, e.g.
			// by a host's manual "Generate match" between our list and this
			// call) are both expected under concurrency, not real failures —
			// log at a lower level than an unexpected error and move on to
			// the next court rather than aborting the whole session.
			if appErr, ok := apperr.As(err); ok && (appErr.Code == apperr.CodeInsufficientPlayers || appErr.Code == apperr.CodeConflict) {
				p.logger.Debug("autofill_skip_court", "session_id", sess.ID, "court_id", court.ID, "reason", appErr.Code)
				continue
			}
			p.logger.Warn("autofill_generate_failed", "session_id", sess.ID, "court_id", court.ID, "error", err)
			continue
		}
		p.logger.Info("autofill_match_generated", "session_id", sess.ID, "court_id", court.ID, "match_id", m.ID)

		// Every generated match starts immediately — same behavior as the
		// host-triggered POST /matches/generate handler — so nobody sits on
		// a court waiting for someone to tap "Start". A failure here leaves
		// the match CREATED; the host can still start it by hand.
		if _, err := p.matches.StartMatch(ctx, m.ID); err != nil {
			p.logger.Warn("autofill_start_failed", "session_id", sess.ID, "court_id", court.ID, "match_id", m.ID, "error", err)
		}
		waiting -= minPlayersPerMatch
	}
}
