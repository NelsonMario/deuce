package match_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"deuce/backend/internal/club"
	"deuce/backend/internal/lock"
	"deuce/backend/internal/match"
	"deuce/backend/internal/player"
	"deuce/backend/internal/session"
	"deuce/backend/internal/testutil"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type fixture struct {
	pool     *pgxpool.Pool
	clubs    club.Repository
	players  player.Repository
	sessions session.Repository
	matches  *match.Service
}

func newFixture(t *testing.T) fixture {
	pool := testutil.NewTestPool(t)
	logger := discardLogger()
	return fixture{
		pool:     pool,
		clubs:    club.NewRepository(pool),
		players:  player.NewRepository(pool),
		sessions: session.NewRepository(pool),
		matches:  match.NewService(pool, match.NewReadRepository(pool), lock.NoopLocker{}, logger),
	}
}

// seedSessionWithPlayers creates a club, a session, one court, and n players
// already in WAITING state with the given gender split (males first).
func (f fixture) seedSessionWithPlayers(t *testing.T, males, females int) (sessionID, courtID uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	host, err := f.players.Create(ctx, "Host", player.Male)
	if err != nil {
		t.Fatalf("create host: %v", err)
	}
	if _, err := f.players.CreateRating(ctx, host.ID, player.InitialRating); err != nil {
		t.Fatalf("create host rating: %v", err)
	}
	c, err := f.clubs.CreateClub(ctx, "Test Club", host.ID, uniqueJoinCode())
	if err != nil {
		t.Fatalf("create club: %v", err)
	}

	sess, err := f.sessions.CreateSession(ctx, c.ID, "Test Session", session.AssignmentAutomatic)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	court, err := f.sessions.CreateCourt(ctx, sess.ID, "Court 1")
	if err != nil {
		t.Fatalf("create court: %v", err)
	}

	addPlayers := func(n int, gender player.Gender) {
		for i := 0; i < n; i++ {
			p, err := f.players.Create(ctx, fmt.Sprintf("%s-%d", gender, i), gender)
			if err != nil {
				t.Fatalf("create player: %v", err)
			}
			if _, err := f.players.CreateRating(ctx, p.ID, player.InitialRating); err != nil {
				t.Fatalf("create rating: %v", err)
			}
			if _, err := f.sessions.AddSessionPlayer(ctx, sess.ID, p.ID); err != nil {
				t.Fatalf("add session player: %v", err)
			}
		}
	}
	addPlayers(males, player.Male)
	addPlayers(females, player.Female)

	return sess.ID, court.ID
}

var joinCodeCounter int

func uniqueJoinCode() string {
	joinCodeCounter++
	return fmt.Sprintf("CODE%04d", joinCodeCounter)
}

func TestIntegration_GenerateStartFinish_UpdatesRatingsAndReleasesCourt(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	sessionID, courtID := f.seedSessionWithPlayers(t, 4, 0)

	m, err := f.matches.GenerateAutomatic(ctx, match.GenerateInput{
		SessionID: sessionID, CourtID: courtID, Format: match.MenDoubles,
	})
	if err != nil {
		t.Fatalf("generate automatic: %v", err)
	}
	if m.Status != match.StatusCreated {
		t.Fatalf("expected CREATED, got %s", m.Status)
	}

	court, err := f.sessions.GetCourt(ctx, courtID)
	if err != nil {
		t.Fatalf("get court: %v", err)
	}
	if court.Status != session.CourtPlaying {
		t.Fatalf("expected court PLAYING after generation, got %s", court.Status)
	}

	started, err := f.matches.StartMatch(ctx, m.ID)
	if err != nil {
		t.Fatalf("start match: %v", err)
	}
	if started.Status != match.StatusPlaying || started.StartedAt == nil {
		t.Fatalf("expected PLAYING with started_at set, got %+v", started)
	}

	finished, err := f.matches.FinishMatch(ctx, match.FinishInput{MatchID: m.ID, ScoreA: 21, ScoreB: 15})
	if err != nil {
		t.Fatalf("finish match: %v", err)
	}
	if finished.Status != match.StatusFinished || finished.EndedAt == nil {
		t.Fatalf("expected FINISHED with ended_at set, got %+v", finished)
	}
	if finished.Winner == nil || *finished.Winner != match.TeamA {
		t.Fatalf("expected team A to win, got %+v", finished.Winner)
	}

	court, err = f.sessions.GetCourt(ctx, courtID)
	if err != nil {
		t.Fatalf("get court: %v", err)
	}
	if court.Status != session.CourtAvailable {
		t.Fatalf("expected court released to AVAILABLE, got %s", court.Status)
	}

	players, err := f.matches.ListPlayers(ctx, m.ID)
	if err != nil {
		t.Fatalf("list match players: %v", err)
	}
	for _, p := range players {
		if p.RatingAfter == nil || p.RatingChange == nil {
			t.Fatalf("expected rating_after/rating_change to be set for player %s", *p.PlayerID)
		}
		if p.Team == match.TeamA && *p.RatingChange <= 0 {
			t.Fatalf("expected team A (winner) to gain rating, got %v", *p.RatingChange)
		}
		if p.Team == match.TeamB && *p.RatingChange >= 0 {
			t.Fatalf("expected team B (loser) to lose rating, got %v", *p.RatingChange)
		}
	}
}

func TestIntegration_GenerateAutomatic_InsufficientPlayers(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	sessionID, courtID := f.seedSessionWithPlayers(t, 2, 0)

	_, err := f.matches.GenerateAutomatic(ctx, match.GenerateInput{
		SessionID: sessionID, CourtID: courtID, Format: match.MenDoubles,
	})
	if err == nil {
		t.Fatal("expected error for insufficient players")
	}
}

func TestIntegration_ConcurrentGeneration_NeverDoubleBooksAPlayer(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	// Exactly 8 WAITING players and 2 courts: two concurrent generation
	// requests should each get a disjoint set of 4 players. A third
	// concurrent request should fail with insufficient players.
	sessionID, court1 := f.seedSessionWithPlayers(t, 8, 0)
	court2, err := f.sessions.CreateCourt(ctx, sessionID, "Court 2")
	if err != nil {
		t.Fatalf("create second court: %v", err)
	}

	courts := []uuid.UUID{court1, court2.ID}
	var wg sync.WaitGroup
	results := make([]*match.Match, len(courts))
	errs := make([]error, len(courts))

	for i, courtID := range courts {
		wg.Add(1)
		go func(i int, courtID uuid.UUID) {
			defer wg.Done()
			m, err := f.matches.GenerateAutomatic(ctx, match.GenerateInput{
				SessionID: sessionID, CourtID: courtID, Format: match.MenDoubles,
			})
			if err != nil {
				errs[i] = err
				return
			}
			results[i] = &m
		}(i, courtID)
	}
	wg.Wait()

	seen := map[uuid.UUID]bool{}
	successCount := 0
	for i, m := range results {
		if errs[i] != nil {
			t.Fatalf("unexpected error generating match %d: %v", i, errs[i])
		}
		successCount++
		players, err := f.matches.ListPlayers(ctx, m.ID)
		if err != nil {
			t.Fatalf("list players: %v", err)
		}
		if len(players) != 4 {
			t.Fatalf("expected 4 players in match, got %d", len(players))
		}
		for _, p := range players {
			playerID := *p.PlayerID
			if seen[playerID] {
				t.Fatalf("player %s was assigned to more than one match — concurrency safety violated", playerID)
			}
			seen[playerID] = true
		}
	}
	if successCount != 2 {
		t.Fatalf("expected both concurrent generations to succeed with disjoint player pools, got %d successes", successCount)
	}
	if len(seen) != 8 {
		t.Fatalf("expected all 8 players to be assigned exactly once, got %d unique assignments", len(seen))
	}

	// A third request now has zero WAITING players left.
	_, err = f.matches.GenerateAutomatic(ctx, match.GenerateInput{
		SessionID: sessionID, CourtID: court1, Format: match.MenDoubles,
	})
	if err == nil {
		t.Fatal("expected error: court1 already PLAYING")
	}
}
