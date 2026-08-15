// Package session implements badminton sessions, session-player rotation
// state, and courts.
package session

import (
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusNotStarted Status = "NOT_STARTED"
	StatusActive     Status = "ACTIVE"
	StatusFinished   Status = "FINISHED"
)

type AssignmentMode string

const (
	AssignmentAutomatic AssignmentMode = "AUTOMATIC"
	AssignmentManual    AssignmentMode = "MANUAL"
)

func (m AssignmentMode) Valid() bool {
	return m == AssignmentAutomatic || m == AssignmentManual
}

type PlayerStatus string

const (
	PlayerWaiting PlayerStatus = "WAITING"
	PlayerPlaying PlayerStatus = "PLAYING"
	PlayerBreak   PlayerStatus = "BREAK"
	PlayerEnded   PlayerStatus = "ENDED"
)

func (s PlayerStatus) Valid() bool {
	switch s {
	case PlayerWaiting, PlayerPlaying, PlayerBreak, PlayerEnded:
		return true
	}
	return false
}

// CanTransitionTo enforces the state machine from spec section 9:
//
//	JOIN -> WAITING
//	WAITING -> PLAYING
//	PLAYING -> WAITING | BREAK | ENDED   (backend-driven, via match finish)
//	BREAK -> WAITING
//	ENDED is terminal
//
// Host/player-initiated transitions via the status PATCH endpoint may only
// move WAITING<->BREAK and WAITING/BREAK->ENDED; PLAYING is exclusively
// backend-controlled through match generation/finish.
func (from PlayerStatus) CanTransitionTo(to PlayerStatus) bool {
	switch from {
	case PlayerWaiting:
		return to == PlayerBreak || to == PlayerEnded || to == PlayerPlaying
	case PlayerBreak:
		return to == PlayerWaiting || to == PlayerEnded
	case PlayerPlaying:
		return to == PlayerWaiting || to == PlayerBreak || to == PlayerEnded
	case PlayerEnded:
		return false
	}
	return false
}

type CourtStatus string

const (
	CourtAvailable CourtStatus = "AVAILABLE"
	CourtPlaying   CourtStatus = "PLAYING"
)

type Session struct {
	ID              uuid.UUID
	ClubID          uuid.UUID
	Name            string
	Status          Status
	AssignmentMode  AssignmentMode
	AutoFillEnabled bool
	StartedAt       *time.Time
	EndedAt         *time.Time
	CreatedAt       time.Time
}

type Player struct {
	ID                        uuid.UUID
	SessionID                 uuid.UUID
	PlayerID                  uuid.UUID
	Status                    PlayerStatus
	WaitingStartedAt          time.Time
	AccumulatedWaitingSeconds int64
	MatchesPlayed             int32
	Wins                      int32
	Losses                    int32
}

// CurrentWaitingSeconds returns the total time this player has spent WAITING
// in the session, including time accrued in the current WAITING period if
// still WAITING.
func (p Player) CurrentWaitingSeconds(now time.Time) float64 {
	total := float64(p.AccumulatedWaitingSeconds)
	if p.Status == PlayerWaiting {
		total += now.Sub(p.WaitingStartedAt).Seconds()
	}
	return total
}

type Court struct {
	ID        uuid.UUID
	SessionID uuid.UUID
	Name      string
	Status    CourtStatus
}
