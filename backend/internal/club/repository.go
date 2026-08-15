package club

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"deuce/backend/internal/database/db"
	"deuce/backend/internal/player"
)

var ErrNotFound = errors.New("club: not found")

type Repository interface {
	CreateClub(ctx context.Context, name string, hostPlayerID uuid.UUID, joinCode string) (Club, error)
	GetByID(ctx context.Context, id uuid.UUID) (Club, error)
	GetByJoinCode(ctx context.Context, joinCode string) (Club, error)
	AddMember(ctx context.Context, clubID, playerID uuid.UUID, role Role) (Member, error)
	GetMember(ctx context.Context, clubID, playerID uuid.UUID) (Member, error)
	// FindMemberPlayerByName returns the player identity of a club member whose
	// display name matches case-insensitively, or ErrNotFound if no member has
	// that name. Guest registration uses it to reuse an existing player row
	// instead of creating a duplicate for a guest the host re-registers.
	FindMemberPlayerByName(ctx context.Context, clubID uuid.UUID, displayName string) (player.Player, error)
	ListMembers(ctx context.Context, clubID uuid.UUID) ([]Member, error)
	SetMemberRole(ctx context.Context, clubID, playerID uuid.UUID, role Role) (Member, error)
}

type pgRepository struct {
	q *db.Queries
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &pgRepository{q: db.New(pool)}
}

func (r *pgRepository) CreateClub(ctx context.Context, name string, hostPlayerID uuid.UUID, joinCode string) (Club, error) {
	row, err := r.q.CreateClub(ctx, db.CreateClubParams{
		Name:         name,
		HostPlayerID: hostPlayerID,
		JoinCode:     joinCode,
	})
	if err != nil {
		return Club{}, fmt.Errorf("create club: %w", err)
	}
	return toClub(row), nil
}

func (r *pgRepository) GetByID(ctx context.Context, id uuid.UUID) (Club, error) {
	row, err := r.q.GetClubByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Club{}, ErrNotFound
		}
		return Club{}, fmt.Errorf("get club: %w", err)
	}
	return toClub(row), nil
}

func (r *pgRepository) GetByJoinCode(ctx context.Context, joinCode string) (Club, error) {
	row, err := r.q.GetClubByJoinCode(ctx, joinCode)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Club{}, ErrNotFound
		}
		return Club{}, fmt.Errorf("get club by join code: %w", err)
	}
	return toClub(row), nil
}

func (r *pgRepository) AddMember(ctx context.Context, clubID, playerID uuid.UUID, role Role) (Member, error) {
	row, err := r.q.AddClubMember(ctx, db.AddClubMemberParams{
		ClubID:   clubID,
		PlayerID: playerID,
		Role:     db.ClubMemberRole(role),
	})
	if err != nil {
		return Member{}, fmt.Errorf("add club member: %w", err)
	}
	return toMember(row), nil
}

func (r *pgRepository) GetMember(ctx context.Context, clubID, playerID uuid.UUID) (Member, error) {
	row, err := r.q.GetClubMember(ctx, db.GetClubMemberParams{ClubID: clubID, PlayerID: playerID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Member{}, ErrNotFound
		}
		return Member{}, fmt.Errorf("get club member: %w", err)
	}
	return toMember(row), nil
}

func (r *pgRepository) FindMemberPlayerByName(ctx context.Context, clubID uuid.UUID, displayName string) (player.Player, error) {
	row, err := r.q.GetClubMemberPlayerByName(ctx, db.GetClubMemberPlayerByNameParams{
		ClubID: clubID,
		Lower:  displayName,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return player.Player{}, ErrNotFound
		}
		return player.Player{}, fmt.Errorf("find member player by name: %w", err)
	}
	return toPlayer(row), nil
}

func (r *pgRepository) ListMembers(ctx context.Context, clubID uuid.UUID) ([]Member, error) {
	rows, err := r.q.ListClubMembers(ctx, clubID)
	if err != nil {
		return nil, fmt.Errorf("list club members: %w", err)
	}
	members := make([]Member, 0, len(rows))
	for _, row := range rows {
		members = append(members, toMember(row))
	}
	return members, nil
}

func (r *pgRepository) SetMemberRole(ctx context.Context, clubID, playerID uuid.UUID, role Role) (Member, error) {
	row, err := r.q.UpdateClubMemberRole(ctx, db.UpdateClubMemberRoleParams{
		ClubID:   clubID,
		PlayerID: playerID,
		Role:     db.ClubMemberRole(role),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Member{}, ErrNotFound
		}
		return Member{}, fmt.Errorf("set club member role: %w", err)
	}
	return toMember(row), nil
}

func toClub(row db.Club) Club {
	return Club{
		ID:           row.ID,
		Name:         row.Name,
		HostPlayerID: row.HostPlayerID,
		JoinCode:     row.JoinCode,
		CreatedAt:    row.CreatedAt.Time,
		UpdatedAt:    row.UpdatedAt.Time,
	}
}

func toMember(row db.ClubMember) Member {
	return Member{
		ID:       row.ID,
		ClubID:   row.ClubID,
		PlayerID: row.PlayerID,
		Role:     Role(row.Role),
		JoinedAt: row.JoinedAt.Time,
	}
}

func toPlayer(row db.Player) player.Player {
	return player.Player{
		ID:          row.ID,
		DisplayName: row.DisplayName,
		Gender:      player.Gender(row.Gender),
		CreatedAt:   row.CreatedAt.Time,
		UpdatedAt:   row.UpdatedAt.Time,
	}
}
