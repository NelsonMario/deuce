package player

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"deuce/backend/internal/database/db"
)

var ErrNotFound = errors.New("player: not found")

const InitialRating = 1000.0

type Repository interface {
	Create(ctx context.Context, displayName string, gender Gender) (Player, error)
	CreateGuest(ctx context.Context, displayName string, gender Gender) (Player, error)
	GetByID(ctx context.Context, id uuid.UUID) (Player, error)
	UpdateProfile(ctx context.Context, id uuid.UUID, displayName string, gender Gender) (Player, error)
	CreateToken(ctx context.Context, playerID uuid.UUID, tokenHash string) error
	GetByTokenHash(ctx context.Context, tokenHash string) (Player, error)
	CreateRating(ctx context.Context, playerID uuid.UUID, rating float64) (Rating, error)
	GetRating(ctx context.Context, playerID uuid.UUID) (Rating, error)
	ListRatingHistory(ctx context.Context, playerID uuid.UUID, limit, offset int32) ([]RatingHistoryEntry, error)
	ListMatches(ctx context.Context, playerID uuid.UUID, limit, offset int32) ([]MatchSummary, error)
	// CleanupStaleGuests deletes guest players not updated since cutoff and
	// not currently in an active/not-started session, returning how many
	// were deleted.
	CleanupStaleGuests(ctx context.Context, cutoff time.Time) (int, error)
}

type pgRepository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &pgRepository{pool: pool}
}

// q builds a non-transactional *db.Queries from the pool. Cheap to call per
// method (it just wraps the existing pool pointer, no new connection) —
// CleanupStaleGuests is the only method that instead needs the pool itself,
// to open an explicit transaction via database.RunInTx.
func (r *pgRepository) q() *db.Queries {
	return db.New(r.pool)
}

func (r *pgRepository) Create(ctx context.Context, displayName string, gender Gender) (Player, error) {
	row, err := r.q().CreatePlayer(ctx, db.CreatePlayerParams{
		DisplayName: displayName,
		Gender:      db.PlayerGender(gender),
	})
	if err != nil {
		return Player{}, fmt.Errorf("create player: %w", err)
	}
	return toPlayer(row), nil
}

func (r *pgRepository) CreateGuest(ctx context.Context, displayName string, gender Gender) (Player, error) {
	row, err := r.q().CreateGuestPlayer(ctx, db.CreateGuestPlayerParams{
		DisplayName: displayName,
		Gender:      db.PlayerGender(gender),
	})
	if err != nil {
		return Player{}, fmt.Errorf("create guest player: %w", err)
	}
	return toPlayer(row), nil
}

func (r *pgRepository) GetByID(ctx context.Context, id uuid.UUID) (Player, error) {
	row, err := r.q().GetPlayer(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Player{}, ErrNotFound
		}
		return Player{}, fmt.Errorf("get player: %w", err)
	}
	return toPlayer(row), nil
}

func (r *pgRepository) UpdateProfile(ctx context.Context, id uuid.UUID, displayName string, gender Gender) (Player, error) {
	row, err := r.q().UpdatePlayerProfile(ctx, db.UpdatePlayerProfileParams{
		ID:          id,
		DisplayName: displayName,
		Gender:      db.PlayerGender(gender),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Player{}, ErrNotFound
		}
		return Player{}, fmt.Errorf("update player profile: %w", err)
	}
	return toPlayer(row), nil
}

func (r *pgRepository) CreateToken(ctx context.Context, playerID uuid.UUID, tokenHash string) error {
	_, err := r.q().CreatePlayerToken(ctx, db.CreatePlayerTokenParams{
		PlayerID:  playerID,
		TokenHash: tokenHash,
	})
	if err != nil {
		return fmt.Errorf("create player token: %w", err)
	}
	return nil
}

func (r *pgRepository) GetByTokenHash(ctx context.Context, tokenHash string) (Player, error) {
	row, err := r.q().GetPlayerByTokenHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Player{}, ErrNotFound
		}
		return Player{}, fmt.Errorf("get player by token: %w", err)
	}
	return toPlayer(row), nil
}

func (r *pgRepository) CreateRating(ctx context.Context, playerID uuid.UUID, rating float64) (Rating, error) {
	row, err := r.q().CreatePlayerRating(ctx, db.CreatePlayerRatingParams{
		PlayerID: playerID,
		Rating:   rating,
	})
	if err != nil {
		return Rating{}, fmt.Errorf("create player rating: %w", err)
	}
	return toRating(row), nil
}

func (r *pgRepository) GetRating(ctx context.Context, playerID uuid.UUID) (Rating, error) {
	row, err := r.q().GetPlayerRating(ctx, playerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Rating{}, ErrNotFound
		}
		return Rating{}, fmt.Errorf("get player rating: %w", err)
	}
	return toRating(row), nil
}

func (r *pgRepository) ListRatingHistory(ctx context.Context, playerID uuid.UUID, limit, offset int32) ([]RatingHistoryEntry, error) {
	rows, err := r.q().ListRatingHistoryByPlayer(ctx, db.ListRatingHistoryByPlayerParams{
		PlayerID: playerID,
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		return nil, fmt.Errorf("list rating history: %w", err)
	}
	entries := make([]RatingHistoryEntry, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, RatingHistoryEntry{
			ID:           row.ID,
			PlayerID:     row.PlayerID,
			MatchID:      row.MatchID,
			RatingBefore: row.RatingBefore,
			RatingAfter:  row.RatingAfter,
			RatingChange: row.RatingChange,
			CreatedAt:    row.CreatedAt.Time,
		})
	}
	return entries, nil
}

func (r *pgRepository) ListMatches(ctx context.Context, playerID uuid.UUID, limit, offset int32) ([]MatchSummary, error) {
	rows, err := r.q().ListMatchesByPlayer(ctx, db.ListMatchesByPlayerParams{
		PlayerID:  playerID,
		RowLimit:  limit,
		RowOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("list matches by player: %w", err)
	}
	summaries := make([]MatchSummary, 0, len(rows))
	for _, row := range rows {
		s := MatchSummary{
			MatchID:   row.ID,
			SessionID: row.SessionID,
			Format:    string(row.Format),
			Status:    string(row.Status),
		}
		if row.StartedAt.Valid {
			t := row.StartedAt.Time
			s.StartedAt = &t
		}
		if row.EndedAt.Valid {
			t := row.EndedAt.Time
			s.EndedAt = &t
		}
		if row.ScoreA.Valid {
			v := row.ScoreA.Int32
			s.ScoreA = &v
		}
		if row.ScoreB.Valid {
			v := row.ScoreB.Int32
			s.ScoreB = &v
		}
		if row.Winner.Valid {
			v := string(row.Winner.MatchWinner)
			s.Winner = &v
		}
		summaries = append(summaries, s)
	}
	return summaries, nil
}

// CleanupStaleGuests deletes stale guests in a single statement. It doesn't
// need an explicit transaction: match_players.player_id and
// session_players.player_id are both ON DELETE SET NULL (see migration
// 000004), so the delete alone atomically clears those links while leaving
// the match/session rows themselves — and everything else that isn't
// guest-owned state — untouched.
func (r *pgRepository) CleanupStaleGuests(ctx context.Context, cutoff time.Time) (int, error) {
	ids, err := r.q().DeleteStaleGuests(ctx, pgtype.Timestamptz{Time: cutoff, Valid: true})
	if err != nil {
		return 0, fmt.Errorf("delete stale guests: %w", err)
	}
	return len(ids), nil
}

func toPlayer(row db.Player) Player {
	return Player{
		ID:          row.ID,
		DisplayName: row.DisplayName,
		Gender:      Gender(row.Gender),
		IsGuest:     row.IsGuest,
		CreatedAt:   row.CreatedAt.Time,
		UpdatedAt:   row.UpdatedAt.Time,
	}
}

func toRating(row db.PlayerRating) Rating {
	return Rating{
		PlayerID:  row.PlayerID,
		Rating:    row.Rating,
		UpdatedAt: row.UpdatedAt.Time,
	}
}
