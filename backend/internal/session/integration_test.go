package session_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"deuce/backend/internal/apperr"
	"deuce/backend/internal/auth"
	"deuce/backend/internal/club"
	"deuce/backend/internal/player"
	"deuce/backend/internal/session"
	"deuce/backend/internal/testutil"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type fixture struct {
	pool           *pgxpool.Pool
	clubRepo       club.Repository
	playerRepo     player.Repository
	sessionRepo    session.Repository
	clubService    *club.Service
	sessionService *session.Service
}

func newFixture(t *testing.T) fixture {
	pool := testutil.NewTestPool(t)
	logger := discardLogger()

	clubRepo := club.NewRepository(pool)
	playerRepo := player.NewRepository(pool)
	sessionRepo := session.NewRepository(pool)
	hasher := auth.NewHasher("test-secret")

	return fixture{
		pool:        pool,
		clubRepo:    clubRepo,
		playerRepo:  playerRepo,
		sessionRepo: sessionRepo,
		// devices is unused by the code paths exercised below (RequireHost
		// and SetAssignmentMode never touch it), so nil is safe here — same
		// approach used by match.newLockedService for unused dependencies.
		clubService:    club.NewService(clubRepo, playerRepo, hasher, nil, 8, logger),
		sessionService: session.NewService(sessionRepo, clubRepo, playerRepo, hasher, nil, logger),
	}
}

var joinCodeCounter int

func uniqueJoinCode() string {
	joinCodeCounter++
	return fmt.Sprintf("CODE%04d", joinCodeCounter)
}

func TestIntegration_CreateSession_AddsHostAsWaitingPlayer(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	host, err := f.playerRepo.Create(ctx, "Host", player.Male)
	if err != nil {
		t.Fatalf("create host: %v", err)
	}
	c, err := f.clubRepo.CreateClub(ctx, "Test Club", host.ID, uniqueJoinCode())
	if err != nil {
		t.Fatalf("create club: %v", err)
	}
	if _, err := f.clubRepo.AddMember(ctx, c.ID, host.ID, club.RoleHost); err != nil {
		t.Fatalf("add host member: %v", err)
	}

	sess, err := f.sessionService.CreateSession(ctx, session.CreateSessionInput{
		ClubID:       c.ID,
		HostPlayerID: host.ID,
		Name:         "Host Plays Too",
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	players, err := f.sessionService.ListPlayers(ctx, sess.ID)
	if err != nil {
		t.Fatalf("list players: %v", err)
	}
	if len(players) != 1 {
		t.Fatalf("expected exactly one session player, got %d", len(players))
	}
	if players[0].PlayerID == nil || *players[0].PlayerID != host.ID {
		t.Fatalf("expected host player %s, got %v", host.ID, players[0].PlayerID)
	}
	if players[0].Status != session.PlayerWaiting {
		t.Fatalf("expected host to start WAITING, got %s", players[0].Status)
	}
}

// TestIntegration_SetAssignmentMode_ActiveSession_HostOnly exercises the same
// two-step flow the PATCH /sessions/:sessionId/assignment-mode handler
// performs: club.Service.RequireHost as the authorization gate, then
// session.Service.SetAssignmentMode. It verifies the mode can be flipped
// mid-session (session is ACTIVE, not just NOT_STARTED) for the host, and
// that a non-host caller is rejected with Forbidden before any mutation.
func TestIntegration_SetAssignmentMode_ActiveSession_HostOnly(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	host, err := f.playerRepo.Create(ctx, "Host", player.Male)
	if err != nil {
		t.Fatalf("create host: %v", err)
	}
	nonHost, err := f.playerRepo.Create(ctx, "NonHost", player.Male)
	if err != nil {
		t.Fatalf("create non-host: %v", err)
	}

	c, err := f.clubRepo.CreateClub(ctx, "Test Club", host.ID, uniqueJoinCode())
	if err != nil {
		t.Fatalf("create club: %v", err)
	}
	if _, err := f.clubRepo.AddMember(ctx, c.ID, host.ID, club.RoleHost); err != nil {
		t.Fatalf("add host member: %v", err)
	}
	if _, err := f.clubRepo.AddMember(ctx, c.ID, nonHost.ID, club.RolePlayer); err != nil {
		t.Fatalf("add non-host member: %v", err)
	}

	sess, err := f.sessionRepo.CreateSession(ctx, c.ID, "Test Session", session.AssignmentAutomatic)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	sess, err = f.sessionRepo.StartSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("start session: %v", err)
	}
	if sess.Status != session.StatusActive {
		t.Fatalf("expected session to be ACTIVE, got %s", sess.Status)
	}

	// Non-host caller: authorization must fail before any mode change.
	if err := f.clubService.RequireHost(ctx, c.ID, nonHost.ID); err == nil {
		t.Fatal("expected non-host caller to be rejected")
	} else if appErr, ok := apperr.As(err); !ok || appErr.Code != apperr.CodeForbidden {
		t.Fatalf("expected Forbidden, got %v", err)
	}

	unchanged, err := f.sessionService.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if unchanged.AssignmentMode != session.AssignmentAutomatic {
		t.Fatalf("expected assignment mode to remain AUTOMATIC after rejected attempt, got %s", unchanged.AssignmentMode)
	}

	// Host caller: authorization passes, and the toggle succeeds while the
	// session is still ACTIVE.
	if err := f.clubService.RequireHost(ctx, c.ID, host.ID); err != nil {
		t.Fatalf("expected host to be authorized: %v", err)
	}
	updated, err := f.sessionService.SetAssignmentMode(ctx, sess.ID, session.AssignmentManual)
	if err != nil {
		t.Fatalf("set assignment mode: %v", err)
	}
	if updated.Status != session.StatusActive {
		t.Fatalf("expected session to remain ACTIVE, got %s", updated.Status)
	}
	if updated.AssignmentMode != session.AssignmentManual {
		t.Fatalf("expected MANUAL, got %s", updated.AssignmentMode)
	}
}

func TestIntegration_SetAssignmentMode_InvalidMode(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	host, err := f.playerRepo.Create(ctx, "Host", player.Male)
	if err != nil {
		t.Fatalf("create host: %v", err)
	}
	c, err := f.clubRepo.CreateClub(ctx, "Test Club", host.ID, uniqueJoinCode())
	if err != nil {
		t.Fatalf("create club: %v", err)
	}
	sess, err := f.sessionRepo.CreateSession(ctx, c.ID, "Test Session", session.AssignmentAutomatic)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	_, err = f.sessionService.SetAssignmentMode(ctx, sess.ID, session.AssignmentMode("BOGUS"))
	if err == nil {
		t.Fatal("expected error for invalid assignment_mode")
	}
	appErr, ok := apperr.As(err)
	if !ok || appErr.Code != apperr.CodeValidation {
		t.Fatalf("expected Validation error, got %v", err)
	}
}

// TestIntegration_SetAutoFillEnabled_ActiveSession_HostOnly exercises the
// same two-step flow the PATCH /sessions/:sessionId/auto-fill handler
// performs: club.Service.RequireHost as the authorization gate, then
// session.Service.SetAutoFillEnabled. It verifies the flag can be toggled
// mid-session (session is ACTIVE, not just NOT_STARTED) for the host, and
// that a non-host caller is rejected with Forbidden before any mutation.
func TestIntegration_SetAutoFillEnabled_ActiveSession_HostOnly(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	host, err := f.playerRepo.Create(ctx, "Host", player.Male)
	if err != nil {
		t.Fatalf("create host: %v", err)
	}
	nonHost, err := f.playerRepo.Create(ctx, "NonHost", player.Male)
	if err != nil {
		t.Fatalf("create non-host: %v", err)
	}

	c, err := f.clubRepo.CreateClub(ctx, "Test Club", host.ID, uniqueJoinCode())
	if err != nil {
		t.Fatalf("create club: %v", err)
	}
	if _, err := f.clubRepo.AddMember(ctx, c.ID, host.ID, club.RoleHost); err != nil {
		t.Fatalf("add host member: %v", err)
	}
	if _, err := f.clubRepo.AddMember(ctx, c.ID, nonHost.ID, club.RolePlayer); err != nil {
		t.Fatalf("add non-host member: %v", err)
	}

	sess, err := f.sessionRepo.CreateSession(ctx, c.ID, "Test Session", session.AssignmentAutomatic)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if !sess.AutoFillEnabled {
		t.Fatalf("expected auto_fill_enabled to default true, got %v", sess.AutoFillEnabled)
	}
	sess, err = f.sessionRepo.StartSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("start session: %v", err)
	}
	if sess.Status != session.StatusActive {
		t.Fatalf("expected session to be ACTIVE, got %s", sess.Status)
	}

	// Non-host caller: authorization must fail before any mutation.
	if err := f.clubService.RequireHost(ctx, c.ID, nonHost.ID); err == nil {
		t.Fatal("expected non-host caller to be rejected")
	} else if appErr, ok := apperr.As(err); !ok || appErr.Code != apperr.CodeForbidden {
		t.Fatalf("expected Forbidden, got %v", err)
	}

	unchanged, err := f.sessionService.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if !unchanged.AutoFillEnabled {
		t.Fatalf("expected auto_fill_enabled to remain true after rejected attempt, got %v", unchanged.AutoFillEnabled)
	}

	// Host caller: authorization passes, and the toggle succeeds while the
	// session is still ACTIVE.
	if err := f.clubService.RequireHost(ctx, c.ID, host.ID); err != nil {
		t.Fatalf("expected host to be authorized: %v", err)
	}
	updated, err := f.sessionService.SetAutoFillEnabled(ctx, sess.ID, false)
	if err != nil {
		t.Fatalf("set auto fill enabled: %v", err)
	}
	if updated.Status != session.StatusActive {
		t.Fatalf("expected session to remain ACTIVE, got %s", updated.Status)
	}
	if updated.AutoFillEnabled {
		t.Fatalf("expected auto_fill_enabled to be false, got %v", updated.AutoFillEnabled)
	}

	// Toggling back on works too.
	reEnabled, err := f.sessionService.SetAutoFillEnabled(ctx, sess.ID, true)
	if err != nil {
		t.Fatalf("re-enable auto fill: %v", err)
	}
	if !reEnabled.AutoFillEnabled {
		t.Fatalf("expected auto_fill_enabled to be true again, got %v", reEnabled.AutoFillEnabled)
	}
}

func TestIntegration_SetAutoFillEnabled_UnknownSession(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	_, err := f.sessionService.SetAutoFillEnabled(ctx, uuid.New(), false)
	if err == nil {
		t.Fatal("expected error for unknown session")
	}
	appErr, ok := apperr.As(err)
	if !ok || appErr.Code != apperr.CodeNotFound {
		t.Fatalf("expected NotFound, got %v", err)
	}
}
