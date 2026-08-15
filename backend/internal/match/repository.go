package match

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"deuce/backend/internal/database/db"
)

var ErrNotFound = errors.New("match: not found")

// ReadRepository serves read-only queries outside of the concurrency-critical
// transaction (listing, get-by-id). The transactional generation/finish flows
// are implemented directly in Service against db.Queries — see service.go for
// why: they touch courts, session_players, matches, match_players and
// player_ratings together atomically, and a generic per-table repository
// abstraction would only obscure that single, critical transaction boundary.
type ReadRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (Match, error)
	ListBySession(ctx context.Context, sessionID uuid.UUID) ([]Match, error)
	ListPlayers(ctx context.Context, matchID uuid.UUID) ([]Player, error)
	ListPlayersBySession(ctx context.Context, sessionID uuid.UUID) (map[uuid.UUID][]Player, error)
}

type pgReadRepository struct {
	q *db.Queries
}

func NewReadRepository(pool *pgxpool.Pool) ReadRepository {
	return &pgReadRepository{q: db.New(pool)}
}

func (r *pgReadRepository) GetByID(ctx context.Context, id uuid.UUID) (Match, error) {
	row, err := r.q.GetMatchByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Match{}, ErrNotFound
		}
		return Match{}, fmt.Errorf("get match: %w", err)
	}
	m := toMatch(row)
	players, err := r.ListPlayers(ctx, id)
	if err == nil {
		pIDs := make([]uuid.UUID, 0, len(players))
		for _, p := range players {
			if p.PlayerID != nil {
				pIDs = append(pIDs, *p.PlayerID)
			}
		}
		m.Players = pIDs
	}
	return m, nil
}

func (r *pgReadRepository) ListBySession(ctx context.Context, sessionID uuid.UUID) ([]Match, error) {
	rows, err := r.q.ListMatchesBySession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list matches: %w", err)
	}
	playerMap, _ := r.ListPlayersBySession(ctx, sessionID)
	matches := make([]Match, 0, len(rows))
	for _, row := range rows {
		m := toMatch(row)
		if players, ok := playerMap[m.ID]; ok {
			pIDs := make([]uuid.UUID, 0, len(players))
			for _, p := range players {
				if p.PlayerID != nil {
					pIDs = append(pIDs, *p.PlayerID)
				}
			}
			m.Players = pIDs
		}
		matches = append(matches, m)
	}
	return matches, nil
}

func (r *pgReadRepository) ListPlayers(ctx context.Context, matchID uuid.UUID) ([]Player, error) {
	rows, err := r.q.ListMatchPlayers(ctx, matchID)
	if err != nil {
		return nil, fmt.Errorf("list match players: %w", err)
	}
	players := make([]Player, 0, len(rows))
	for _, row := range rows {
		players = append(players, toPlayer(row))
	}
	return players, nil
}

func (r *pgReadRepository) ListPlayersBySession(ctx context.Context, sessionID uuid.UUID) (map[uuid.UUID][]Player, error) {
	rows, err := r.q.ListMatchPlayersBySession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list session match players: %w", err)
	}
	out := make(map[uuid.UUID][]Player)
	for _, row := range rows {
		p := toPlayer(row)
		out[row.MatchID] = append(out[row.MatchID], p)
	}
	return out, nil
}

func toMatch(row db.Match) Match {
	m := Match{
		ID:        row.ID,
		SessionID: row.SessionID,
		CourtID:   row.CourtID,
		Format:    Format(row.Format),
		Status:    Status(row.Status),
	}
	if row.StartedAt.Valid {
		t := row.StartedAt.Time
		m.StartedAt = &t
	}
	if row.EndedAt.Valid {
		t := row.EndedAt.Time
		m.EndedAt = &t
	}
	if row.ScoreA.Valid {
		v := row.ScoreA.Int32
		m.ScoreA = &v
	}
	if row.ScoreB.Valid {
		v := row.ScoreB.Int32
		m.ScoreB = &v
	}
	if row.Winner.Valid {
		w := Team(row.Winner.MatchWinner)
		m.Winner = &w
	}
	return m
}

func toPlayer(row db.MatchPlayer) Player {
	p := Player{
		MatchID:      row.MatchID,
		Team:         Team(row.Team),
		RatingBefore: 0,
	}
	if row.PlayerID.Valid {
		id := uuid.UUID(row.PlayerID.Bytes)
		p.PlayerID = &id
	}
	if row.RatingBefore.Valid {
		p.RatingBefore = row.RatingBefore.Float64
	}
	if row.RatingAfter.Valid {
		v := row.RatingAfter.Float64
		p.RatingAfter = &v
	}
	if row.RatingChange.Valid {
		v := row.RatingChange.Float64
		p.RatingChange = &v
	}
	return p
}
