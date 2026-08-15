// Package club implements club creation, membership, and join-code based
// entry.
package club

import (
	"time"

	"github.com/google/uuid"
)

type Role string

const (
	RoleHost   Role = "HOST"
	RolePlayer Role = "PLAYER"
)

type Club struct {
	ID           uuid.UUID
	Name         string
	HostPlayerID uuid.UUID
	JoinCode     string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Member struct {
	ID       uuid.UUID
	ClubID   uuid.UUID
	PlayerID uuid.UUID
	Role     Role
	JoinedAt time.Time
}
