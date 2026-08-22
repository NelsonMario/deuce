package club

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	"deuce/backend/internal/apperr"
	"deuce/backend/internal/auth"
	"deuce/backend/internal/device"
	"deuce/backend/internal/player"
	"deuce/backend/pkg/idgen"
)

type Service struct {
	repo           Repository
	players        player.Repository
	hasher         auth.Hasher
	devices        device.Linker
	joinCodeLength int
	logger         *slog.Logger
}

func NewService(repo Repository, players player.Repository, hasher auth.Hasher, devices device.Linker, joinCodeLength int, logger *slog.Logger) *Service {
	return &Service{repo: repo, players: players, hasher: hasher, devices: devices, joinCodeLength: joinCodeLength, logger: logger}
}

type CreateClubInput struct {
	ClubName        string
	HostDisplayName string
	HostGender      player.Gender
	// DeviceID is optional. When set, this device is linked to the new host
	// player for this club, so a later JoinClub/JoinSession from the same
	// device+club recognizes them instead of creating a new identity.
	DeviceID string
}

type AuthenticatedPlayer struct {
	Player    player.Player
	Token     string
	IsNewJoin bool
}

type CreateClubResult struct {
	Club Club
	Host AuthenticatedPlayer
}

// CreateClub creates a new player as the host, then creates the club with a
// fresh unguessable join code, and issues the host a session token.
func (s *Service) CreateClub(ctx context.Context, in CreateClubInput) (CreateClubResult, error) {
	if in.ClubName == "" {
		return CreateClubResult{}, apperr.Validation("club name is required")
	}
	if !in.HostGender.Valid() {
		return CreateClubResult{}, apperr.Validation("invalid gender")
	}

	host, err := s.players.Create(ctx, in.HostDisplayName, in.HostGender)
	if err != nil {
		return CreateClubResult{}, apperr.Internal(err)
	}
	if _, err := s.players.CreateRating(ctx, host.ID, player.InitialRating); err != nil {
		return CreateClubResult{}, apperr.Internal(err)
	}

	joinCode, err := idgen.JoinCode(s.joinCodeLength)
	if err != nil {
		return CreateClubResult{}, apperr.Internal(err)
	}

	c, err := s.repo.CreateClub(ctx, in.ClubName, host.ID, joinCode)
	if err != nil {
		return CreateClubResult{}, apperr.Internal(err)
	}

	if _, err := s.repo.AddMember(ctx, c.ID, host.ID, RoleHost); err != nil {
		return CreateClubResult{}, apperr.Internal(err)
	}

	token, err := s.issueToken(ctx, host.ID)
	if err != nil {
		return CreateClubResult{}, err
	}

	s.devices.Link(ctx, in.DeviceID, c.ID, host.ID)

	s.logger.Info("club_created", "club_id", c.ID, "host_player_id", host.ID)

	return CreateClubResult{
		Club: c,
		Host: AuthenticatedPlayer{Player: host, Token: token, IsNewJoin: true},
	}, nil
}

// GetByJoinCode resolves a human-typed join code to its club without
// authentication — the unguessable code IS the credential here (same trust
// level as JoinClub itself). Input is normalized so lowercase or padded
// entries still resolve.
func (s *Service) GetByJoinCode(ctx context.Context, joinCode string) (Club, error) {
	code := strings.ToUpper(strings.TrimSpace(joinCode))
	if code == "" {
		return Club{}, apperr.Validation("join code is required")
	}
	c, err := s.repo.GetByJoinCode(ctx, code)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Club{}, apperr.NotFound("club")
		}
		return Club{}, apperr.Internal(err)
	}
	return c, nil
}

type JoinClubInput struct {
	JoinCode    string
	DisplayName string
	Gender      player.Gender
	// DeviceID is optional. When it matches a previous join/create for this
	// club, the caller is recognized as that same player (rating/history
	// carry over, and role — e.g. HOST — is preserved) instead of getting a
	// brand-new identity.
	DeviceID string
}

// JoinClub validates the club join code and either recognizes a returning
// device (see internal/device) or creates a brand-new player identity, adds
// them as a club member, and issues a fresh session token either way.
func (s *Service) JoinClub(ctx context.Context, clubID uuid.UUID, in JoinClubInput) (Club, AuthenticatedPlayer, error) {
	if in.DisplayName == "" {
		return Club{}, AuthenticatedPlayer{}, apperr.Validation("display name is required")
	}
	if !in.Gender.Valid() {
		return Club{}, AuthenticatedPlayer{}, apperr.Validation("invalid gender")
	}
	// Codes are generated uppercase; clients render the input uppercase via
	// CSS but that doesn't change what's actually typed, so accept any case
	// here rather than failing every lowercase entry.
	in.JoinCode = strings.ToUpper(strings.TrimSpace(in.JoinCode))

	c, err := s.repo.GetByID(ctx, clubID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Club{}, AuthenticatedPlayer{}, apperr.NotFound("club")
		}
		return Club{}, AuthenticatedPlayer{}, apperr.Internal(err)
	}
	if c.JoinCode != in.JoinCode {
		return Club{}, AuthenticatedPlayer{}, apperr.Unauthorized("invalid join code")
	}

	p, isNewJoin, err := s.resolvePlayer(ctx, in.DeviceID, c.ID, in.DisplayName, in.Gender)
	if err != nil {
		return Club{}, AuthenticatedPlayer{}, err
	}
	if _, err := s.repo.AddMember(ctx, c.ID, p.ID, RolePlayer); err != nil {
		return Club{}, AuthenticatedPlayer{}, apperr.Internal(err)
	}

	token, err := s.issueToken(ctx, p.ID)
	if err != nil {
		return Club{}, AuthenticatedPlayer{}, err
	}

	s.logger.Info("player_joined", "club_id", c.ID, "player_id", p.ID, "returning", !isNewJoin)

	return c, AuthenticatedPlayer{Player: p, Token: token, IsNewJoin: isNewJoin}, nil
}

// resolvePlayer wraps device.Resolve (host role/membership is preserved by
// AddMember's idempotent upsert regardless of which player identity comes
// back here).
func (s *Service) resolvePlayer(ctx context.Context, deviceID string, clubID uuid.UUID, displayName string, gender player.Gender) (player.Player, bool, error) {
	p, isNewJoin, err := device.Resolve(ctx, s.devices, s.players, deviceID, clubID, displayName, gender)
	if err != nil {
		return player.Player{}, false, apperr.Internal(err)
	}
	return p, isNewJoin, nil
}

func (s *Service) issueToken(ctx context.Context, playerID uuid.UUID) (string, error) {
	raw, err := idgen.PlayerToken()
	if err != nil {
		return "", apperr.Internal(err)
	}
	if err := s.players.CreateToken(ctx, playerID, s.hasher.Hash(raw)); err != nil {
		return "", apperr.Internal(err)
	}
	return raw, nil
}

// RequireHost checks that the given player is the HOST of the club.
func (s *Service) RequireHost(ctx context.Context, clubID, playerID uuid.UUID) error {
	m, err := s.repo.GetMember(ctx, clubID, playerID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return apperr.Forbidden("not a member of this club")
		}
		return apperr.Internal(err)
	}
	if m.Role != RoleHost {
		return apperr.Forbidden("host privileges required")
	}
	return nil
}

func (s *Service) GetClub(ctx context.Context, id uuid.UUID) (Club, error) {
	c, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Club{}, apperr.NotFound("club")
		}
		return Club{}, apperr.Internal(err)
	}
	return c, nil
}

// MyRole reports the given player's membership role in the club — used by
// clients to self-check "am I a host of this club" (e.g. a promoted
// co-host recognizing their own status on a device that didn't create the
// club and so has no other local signal of it).
func (s *Service) MyRole(ctx context.Context, clubID, playerID uuid.UUID) (Role, error) {
	m, err := s.repo.GetMember(ctx, clubID, playerID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return "", apperr.Forbidden("not a member of this club")
		}
		return "", apperr.Internal(err)
	}
	return m.Role, nil
}

// ListMembers returns every member of the club, for a host deciding who to
// promote.
func (s *Service) ListMembers(ctx context.Context, clubID uuid.UUID) ([]Member, error) {
	members, err := s.repo.ListMembers(ctx, clubID)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return members, nil
}

// PromoteToHost grants an existing club member HOST privileges alongside
// (not instead of) the current host(s) — every host-gated check
// (RequireHost, and everything built on it: sessions, courts, matches)
// already authorizes by role, not by clubs.host_player_id, so a promoted
// member gets full host access with no other changes needed anywhere.
// There's no equivalent "demote" — see docs/v2-plan.md.
func (s *Service) PromoteToHost(ctx context.Context, clubID, requestingPlayerID, targetPlayerID uuid.UUID) (Member, error) {
	if err := s.RequireHost(ctx, clubID, requestingPlayerID); err != nil {
		return Member{}, err
	}
	target, err := s.repo.GetMember(ctx, clubID, targetPlayerID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Member{}, apperr.NotFound("player is not a member of this club")
		}
		return Member{}, apperr.Internal(err)
	}
	if target.Role == RoleHost {
		return target, nil
	}
	updated, err := s.repo.SetMemberRole(ctx, clubID, targetPlayerID, RoleHost)
	if err != nil {
		return Member{}, apperr.Internal(err)
	}
	s.logger.Info("member_promoted_to_host", "club_id", clubID, "player_id", targetPlayerID, "by", requestingPlayerID)
	return updated, nil
}
