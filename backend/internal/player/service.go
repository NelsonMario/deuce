package player

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"deuce/backend/internal/apperr"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) GetPlayer(ctx context.Context, id uuid.UUID) (Player, error) {
	p, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Player{}, apperr.NotFound("player")
		}
		return Player{}, apperr.Internal(err)
	}
	return p, nil
}

func (s *Service) GetRating(ctx context.Context, id uuid.UUID) (Rating, error) {
	r, err := s.repo.GetRating(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Rating{}, apperr.NotFound("player rating")
		}
		return Rating{}, apperr.Internal(err)
	}
	return r, nil
}

func (s *Service) ListMatches(ctx context.Context, id uuid.UUID, limit, offset int32) ([]MatchSummary, error) {
	matches, err := s.repo.ListMatches(ctx, id, limit, offset)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return matches, nil
}

func (s *Service) ListRatingHistory(ctx context.Context, id uuid.UUID, limit, offset int32) ([]RatingHistoryEntry, error) {
	entries, err := s.repo.ListRatingHistory(ctx, id, limit, offset)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return entries, nil
}

// CleanupStaleGuests deletes guest players not seen (players.updated_at) in
// retentionDays, skipping any still in a not-started/active session. It
// returns how many were deleted.
func (s *Service) CleanupStaleGuests(ctx context.Context, retentionDays int) (int, error) {
	if retentionDays <= 0 {
		return 0, apperr.Validation("retention_days must be positive")
	}
	cutoff := time.Now().Add(-time.Duration(retentionDays) * 24 * time.Hour)
	deleted, err := s.repo.CleanupStaleGuests(ctx, cutoff)
	if err != nil {
		return 0, apperr.Internal(err)
	}
	return deleted, nil
}
