// Package match implements match generation (automatic and manual),
// lifecycle (start/finish), and ties together the pure matchmaking and
// rating engines with concurrency-safe PostgreSQL transactions.
package match

import (
	"time"

	"github.com/google/uuid"
)

type Format string

const (
	MixedDoubles Format = "MIXED_DOUBLES"
	MenDoubles   Format = "MEN_DOUBLES"
	WomenDoubles Format = "WOMEN_DOUBLES"
)

func (f Format) Valid() bool {
	switch f {
	case MixedDoubles, MenDoubles, WomenDoubles:
		return true
	}
	return false
}

type Status string

const (
	StatusCreated  Status = "CREATED"
	StatusPlaying  Status = "PLAYING"
	StatusFinished Status = "FINISHED"
)

type Team string

const (
	TeamA Team = "A"
	TeamB Team = "B"
)

type Match struct {
	ID        uuid.UUID
	SessionID uuid.UUID
	CourtID   uuid.UUID
	Format    Format
	Status    Status
	StartedAt *time.Time
	EndedAt   *time.Time
	ScoreA    *int32
	ScoreB    *int32
	Winner    *Team
}

type Player struct {
	MatchID      uuid.UUID
	PlayerID     uuid.UUID
	Team         Team
	RatingBefore float64
	RatingAfter  *float64
	RatingChange *float64
}

// Proposal is a preview of an assignment, used for the MANUAL host
// recommendation endpoint (the host may override before confirming).
type Proposal struct {
	Format      Format
	TeamA       [2]uuid.UUID
	TeamB       [2]uuid.UUID
	TeamARating float64
	TeamBRating float64
	RatingDiff  float64
}
