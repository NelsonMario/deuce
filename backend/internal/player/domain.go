// Package player implements player identity, gender, and rating lookups.
// Gender affects only match-format eligibility, never rating.
package player

import (
	"time"

	"github.com/google/uuid"
)

type Gender string

const (
	Male   Gender = "MALE"
	Female Gender = "FEMALE"
)

func (g Gender) Valid() bool {
	return g == Male || g == Female
}

type Player struct {
	ID          uuid.UUID
	DisplayName string
	Gender      Gender
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Rating struct {
	PlayerID  uuid.UUID
	Rating    float64
	UpdatedAt time.Time
}

type MatchSummary struct {
	MatchID   uuid.UUID
	SessionID uuid.UUID
	Format    string
	Status    string
	StartedAt *time.Time
	EndedAt   *time.Time
	ScoreA    *int32
	ScoreB    *int32
	Winner    *string
}

type RatingHistoryEntry struct {
	ID           uuid.UUID
	PlayerID     uuid.UUID
	MatchID      uuid.UUID
	RatingBefore float64
	RatingAfter  float64
	RatingChange float64
	CreatedAt    time.Time
}
