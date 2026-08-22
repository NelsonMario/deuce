package handler

import (
	"github.com/gofiber/fiber/v2"

	"deuce/backend/internal/club"
	"deuce/backend/internal/player"
	"deuce/backend/internal/session"
)

type CreateClubRequest struct {
	ClubName        string `json:"club_name" validate:"required,min=1,max=100"`
	HostDisplayName string `json:"host_display_name" validate:"required,min=1,max=60"`
	HostGender      string `json:"host_gender" validate:"required,oneof=MALE FEMALE"`
	// DeviceID is optional (v2). When provided, this device is linked to the
	// host player for this club so a later join from the same device+club
	// is recognized instead of creating a new identity.
	DeviceID string `json:"device_id" validate:"omitempty,max=100"`
}

type PlayerAuthResponse struct {
	Player PlayerDTO `json:"player"`
	Token  string    `json:"token"`
	// Returning is true when this response reused an existing player
	// identity recognized via a linked device (v2), rather than creating a
	// brand-new one.
	Returning bool `json:"returning"`
}

type ClubDTO struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	HostPlayerID string `json:"host_player_id"`
	JoinCode     string `json:"join_code"`
}

func toClubDTO(c club.Club) ClubDTO {
	return ClubDTO{ID: c.ID.String(), Name: c.Name, HostPlayerID: c.HostPlayerID.String(), JoinCode: c.JoinCode}
}

type CreateClubResponse struct {
	Club ClubDTO            `json:"club"`
	Host PlayerAuthResponse `json:"host"`
}

func (h *Handlers) CreateClub(c *fiber.Ctx) error {
	var req CreateClubRequest
	if err := BindAndValidate(c, &req); err != nil {
		return HandleError(c, err)
	}

	result, err := h.Clubs.CreateClub(c.UserContext(), club.CreateClubInput{
		ClubName:        req.ClubName,
		HostDisplayName: req.HostDisplayName,
		HostGender:      player.Gender(req.HostGender),
		DeviceID:        req.DeviceID,
	})
	if err != nil {
		return HandleError(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(CreateClubResponse{
		Club: toClubDTO(result.Club),
		Host: PlayerAuthResponse{
			Player:    toPlayerDTO(result.Host.Player),
			Token:     result.Host.Token,
			Returning: !result.Host.IsNewJoin,
		},
	})
}

type JoinClubRequest struct {
	JoinCode    string `json:"join_code" validate:"required"`
	DisplayName string `json:"display_name" validate:"required,min=1,max=60"`
	Gender      string `json:"gender" validate:"required,oneof=MALE FEMALE"`
	// DeviceID is optional (v2). See CreateClubRequest.
	DeviceID string `json:"device_id" validate:"omitempty,max=100"`
}

// ResolveClubRequest carries just a club join code — how a player joins
// with nothing but the code the host shared, no session link or club ID.
type ResolveClubRequest struct {
	JoinCode string `json:"join_code" validate:"required,max=12"`
}

// ActiveSessionDTO is the minimal session info the resolve response needs:
// which live session (if any) a club-code joiner should land in.
type ActiveSessionDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type ResolveClubResponse struct {
	Club ClubDTO `json:"club"`
	// ActiveSessions lists the club's ACTIVE sessions, newest first. Empty
	// when nothing is running — the client then drops the player on the
	// club page instead of a specific session.
	ActiveSessions []ActiveSessionDTO `json:"active_sessions"`
}

// ResolveClub looks up a club by join code and reports its active sessions.
// Unauthenticated by design: the join code is the secret, so anyone holding
// it may already join the club; this endpoint only saves them from needing
// a separate club ID or invite link. Rate limiting still applies globally.
func (h *Handlers) ResolveClub(c *fiber.Ctx) error {
	var req ResolveClubRequest
	if err := BindAndValidate(c, &req); err != nil {
		return HandleError(c, err)
	}

	cl, err := h.Clubs.GetByJoinCode(c.UserContext(), req.JoinCode)
	if err != nil {
		return HandleError(c, err)
	}

	sessions, err := h.Sessions.ListByClub(c.UserContext(), cl.ID)
	if err != nil {
		return HandleError(c, err)
	}
	active := make([]ActiveSessionDTO, 0, len(sessions))
	for _, s := range sessions {
		if s.Status != session.StatusActive {
			continue
		}
		active = append(active, ActiveSessionDTO{ID: s.ID.String(), Name: s.Name})
	}

	return c.JSON(ResolveClubResponse{Club: toClubDTO(cl), ActiveSessions: active})
}

type JoinClubResponse struct {
	Club ClubDTO            `json:"club"`
	You  PlayerAuthResponse `json:"you"`
}

func (h *Handlers) JoinClub(c *fiber.Ctx) error {
	clubID, err := ParseUUIDParam(c, "clubId")
	if err != nil {
		return HandleError(c, err)
	}
	var req JoinClubRequest
	if err := BindAndValidate(c, &req); err != nil {
		return HandleError(c, err)
	}

	joinedClub, authed, err := h.Clubs.JoinClub(c.UserContext(), clubID, club.JoinClubInput{
		JoinCode:    req.JoinCode,
		DisplayName: req.DisplayName,
		Gender:      player.Gender(req.Gender),
		DeviceID:    req.DeviceID,
	})
	if err != nil {
		return HandleError(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(JoinClubResponse{
		Club: toClubDTO(joinedClub),
		You: PlayerAuthResponse{
			Player:    toPlayerDTO(authed.Player),
			Token:     authed.Token,
			Returning: !authed.IsNewJoin,
		},
	})
}

func (h *Handlers) GetClub(c *fiber.Ctx) error {
	clubID, err := ParseUUIDParam(c, "clubId")
	if err != nil {
		return HandleError(c, err)
	}
	cl, err := h.Clubs.GetClub(c.UserContext(), clubID)
	if err != nil {
		return HandleError(c, err)
	}
	return c.JSON(toClubDTO(cl))
}

type MyRoleResponse struct {
	Role string `json:"role"`
}

// GetMyRole lets an authenticated caller self-check their own membership
// role in a club — this is how a client recognizes "I'm a host of this
// club" on a device that didn't create it (e.g. a promoted co-host),
// rather than only trusting a local "I created this" flag.
func (h *Handlers) GetMyRole(c *fiber.Ctx) error {
	clubID, err := ParseUUIDParam(c, "clubId")
	if err != nil {
		return HandleError(c, err)
	}
	principal, err := Principal(c)
	if err != nil {
		return HandleError(c, err)
	}
	role, err := h.Clubs.MyRole(c.UserContext(), clubID, principal.PlayerID)
	if err != nil {
		return HandleError(c, err)
	}
	return c.JSON(MyRoleResponse{Role: string(role)})
}

type MemberDTO struct {
	PlayerID string `json:"player_id"`
	Role     string `json:"role"`
	JoinedAt string `json:"joined_at"`
}

func toMemberDTO(m club.Member) MemberDTO {
	return MemberDTO{
		PlayerID: m.PlayerID.String(),
		Role:     string(m.Role),
		JoinedAt: m.JoinedAt.Format(timeFormat),
	}
}

// ListClubMembers is host-only: it's for deciding who to promote, not a
// general public roster.
func (h *Handlers) ListClubMembers(c *fiber.Ctx) error {
	clubID, err := ParseUUIDParam(c, "clubId")
	if err != nil {
		return HandleError(c, err)
	}
	principal, err := Principal(c)
	if err != nil {
		return HandleError(c, err)
	}
	if err := h.Clubs.RequireHost(c.UserContext(), clubID, principal.PlayerID); err != nil {
		return HandleError(c, err)
	}
	members, err := h.Clubs.ListMembers(c.UserContext(), clubID)
	if err != nil {
		return HandleError(c, err)
	}
	dtos := make([]MemberDTO, 0, len(members))
	for _, m := range members {
		dtos = append(dtos, toMemberDTO(m))
	}
	return c.JSON(fiber.Map{"members": dtos})
}

// ListClubSessions is host-only, like ListClubMembers: it's what backs the
// club page's "Your sessions" list, so any host/co-host device sees every
// session created for the club — not just sessions this particular device
// has itself created or joined.
func (h *Handlers) ListClubSessions(c *fiber.Ctx) error {
	clubID, err := ParseUUIDParam(c, "clubId")
	if err != nil {
		return HandleError(c, err)
	}
	principal, err := Principal(c)
	if err != nil {
		return HandleError(c, err)
	}
	if err := h.Clubs.RequireHost(c.UserContext(), clubID, principal.PlayerID); err != nil {
		return HandleError(c, err)
	}
	sessions, err := h.Sessions.ListByClub(c.UserContext(), clubID)
	if err != nil {
		return HandleError(c, err)
	}
	dtos := make([]SessionDTO, 0, len(sessions))
	for _, s := range sessions {
		dtos = append(dtos, toSessionDTO(s))
	}
	return c.JSON(fiber.Map{"sessions": dtos})
}

// PromoteMember grants an existing club member HOST privileges alongside
// the caller (who must already be a host). There's no "demote."
func (h *Handlers) PromoteMember(c *fiber.Ctx) error {
	clubID, err := ParseUUIDParam(c, "clubId")
	if err != nil {
		return HandleError(c, err)
	}
	targetPlayerID, err := ParseUUIDParam(c, "playerId")
	if err != nil {
		return HandleError(c, err)
	}
	principal, err := Principal(c)
	if err != nil {
		return HandleError(c, err)
	}
	member, err := h.Clubs.PromoteToHost(c.UserContext(), clubID, principal.PlayerID, targetPlayerID)
	if err != nil {
		return HandleError(c, err)
	}
	return c.JSON(toMemberDTO(member))
}
