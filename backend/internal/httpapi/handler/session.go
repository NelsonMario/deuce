package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"deuce/backend/internal/player"
	"deuce/backend/internal/session"
)

// requireHostOfSession resolves the session's club and checks that the
// authenticated principal is that club's host.
func (h *Handlers) requireHostOfSession(c *fiber.Ctx, sessionID uuid.UUID) error {
	principal, err := Principal(c)
	if err != nil {
		return err
	}
	sess, err := h.Sessions.GetSession(c.UserContext(), sessionID)
	if err != nil {
		return err
	}
	return h.Clubs.RequireHost(c.UserContext(), sess.ClubID, principal.PlayerID)
}

type CreateSessionRequest struct {
	ClubID         string `json:"club_id" validate:"required,uuid"`
	Name           string `json:"name" validate:"max=100"`
	AssignmentMode string `json:"assignment_mode" validate:"omitempty,oneof=AUTOMATIC MANUAL"`
}

type SessionDTO struct {
	ID              string  `json:"id"`
	ClubID          string  `json:"club_id"`
	Name            string  `json:"name"`
	Status          string  `json:"status"`
	AssignmentMode  string  `json:"assignment_mode"`
	AutoFillEnabled bool    `json:"auto_fill_enabled"`
	StartedAt       *string `json:"started_at,omitempty"`
	EndedAt         *string `json:"ended_at,omitempty"`
}

func toSessionDTO(s session.Session) SessionDTO {
	dto := SessionDTO{
		ID: s.ID.String(), ClubID: s.ClubID.String(), Name: s.Name,
		Status: string(s.Status), AssignmentMode: string(s.AssignmentMode),
		AutoFillEnabled: s.AutoFillEnabled,
	}
	if s.StartedAt != nil {
		v := s.StartedAt.Format(timeFormat)
		dto.StartedAt = &v
	}
	if s.EndedAt != nil {
		v := s.EndedAt.Format(timeFormat)
		dto.EndedAt = &v
	}
	return dto
}

const timeFormat = "2006-01-02T15:04:05Z07:00"

func (h *Handlers) CreateSession(c *fiber.Ctx) error {
	var req CreateSessionRequest
	if err := BindAndValidate(c, &req); err != nil {
		return HandleError(c, err)
	}
	clubID, err := uuid.Parse(req.ClubID)
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

	sess, err := h.Sessions.CreateSession(c.UserContext(), session.CreateSessionInput{
		ClubID: clubID, HostPlayerID: principal.PlayerID, Name: req.Name, AssignmentMode: session.AssignmentMode(req.AssignmentMode),
	})
	if err != nil {
		return HandleError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(toSessionDTO(sess))
}

func (h *Handlers) StartSession(c *fiber.Ctx) error {
	id, err := ParseUUIDParam(c, "sessionId")
	if err != nil {
		return HandleError(c, err)
	}
	if err := h.requireHostOfSession(c, id); err != nil {
		return HandleError(c, err)
	}
	sess, err := h.Sessions.StartSession(c.UserContext(), id)
	if err != nil {
		return HandleError(c, err)
	}
	return c.JSON(toSessionDTO(sess))
}

func (h *Handlers) EndSession(c *fiber.Ctx) error {
	id, err := ParseUUIDParam(c, "sessionId")
	if err != nil {
		return HandleError(c, err)
	}
	if err := h.requireHostOfSession(c, id); err != nil {
		return HandleError(c, err)
	}
	sess, err := h.Sessions.EndSession(c.UserContext(), id)
	if err != nil {
		return HandleError(c, err)
	}
	return c.JSON(toSessionDTO(sess))
}

type SetAssignmentModeRequest struct {
	AssignmentMode string `json:"assignment_mode" validate:"required,oneof=AUTOMATIC MANUAL"`
}

// SetAssignmentMode lets a host flip a session's matchmaking assignment mode
// (AUTOMATIC/MANUAL) mid-session, e.g. between individual matches, not just
// before the session starts.
func (h *Handlers) SetAssignmentMode(c *fiber.Ctx) error {
	id, err := ParseUUIDParam(c, "sessionId")
	if err != nil {
		return HandleError(c, err)
	}
	if err := h.requireHostOfSession(c, id); err != nil {
		return HandleError(c, err)
	}
	var req SetAssignmentModeRequest
	if err := BindAndValidate(c, &req); err != nil {
		return HandleError(c, err)
	}
	sess, err := h.Sessions.SetAssignmentMode(c.UserContext(), id, session.AssignmentMode(req.AssignmentMode))
	if err != nil {
		return HandleError(c, err)
	}
	return c.JSON(toSessionDTO(sess))
}

type SetAutoFillEnabledRequest struct {
	AutoFillEnabled *bool `json:"auto_fill_enabled" validate:"required"`
}

// SetAutoFillEnabled lets a host turn the fully-automatic auto-fill
// background job (see internal/match/autofill.go) on or off for this
// session — e.g. so the host can pause auto-fill without also switching the
// session out of AUTOMATIC assignment mode.
func (h *Handlers) SetAutoFillEnabled(c *fiber.Ctx) error {
	id, err := ParseUUIDParam(c, "sessionId")
	if err != nil {
		return HandleError(c, err)
	}
	if err := h.requireHostOfSession(c, id); err != nil {
		return HandleError(c, err)
	}
	var req SetAutoFillEnabledRequest
	if err := BindAndValidate(c, &req); err != nil {
		return HandleError(c, err)
	}
	sess, err := h.Sessions.SetAutoFillEnabled(c.UserContext(), id, *req.AutoFillEnabled)
	if err != nil {
		return HandleError(c, err)
	}
	return c.JSON(toSessionDTO(sess))
}

type SessionPlayerDTO struct {
	ID             string `json:"id"`
	PlayerID       string `json:"player_id"`
	Status         string `json:"status"`
	MatchesPlayed  int32  `json:"matches_played"`
	Wins           int32  `json:"wins"`
	Losses         int32  `json:"losses"`
	WaitingSeconds int64  `json:"waiting_seconds"`
}

func toSessionPlayerDTO(p session.Player) SessionPlayerDTO {
	return SessionPlayerDTO{
		ID: p.ID.String(), PlayerID: p.PlayerID.String(), Status: string(p.Status),
		MatchesPlayed: p.MatchesPlayed, Wins: p.Wins, Losses: p.Losses,
		WaitingSeconds: int64(p.CurrentWaitingSeconds(time.Now())),
	}
}

type CourtDTO struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

func toCourtDTO(c session.Court) CourtDTO {
	return CourtDTO{ID: c.ID.String(), Name: c.Name, Status: string(c.Status)}
}

type SessionDetailResponse struct {
	Session SessionDTO         `json:"session"`
	Players []SessionPlayerDTO `json:"players"`
	Courts  []CourtDTO         `json:"courts"`
}

func (h *Handlers) GetSession(c *fiber.Ctx) error {
	id, err := ParseUUIDParam(c, "sessionId")
	if err != nil {
		return HandleError(c, err)
	}
	sess, err := h.Sessions.GetSession(c.UserContext(), id)
	if err != nil {
		return HandleError(c, err)
	}
	players, err := h.Sessions.ListPlayers(c.UserContext(), id)
	if err != nil {
		return HandleError(c, err)
	}
	courts, err := h.Sessions.ListCourts(c.UserContext(), id)
	if err != nil {
		return HandleError(c, err)
	}

	playerDTOs := make([]SessionPlayerDTO, 0, len(players))
	for _, p := range players {
		playerDTOs = append(playerDTOs, toSessionPlayerDTO(p))
	}
	courtDTOs := make([]CourtDTO, 0, len(courts))
	for _, ct := range courts {
		courtDTOs = append(courtDTOs, toCourtDTO(ct))
	}

	return c.JSON(SessionDetailResponse{Session: toSessionDTO(sess), Players: playerDTOs, Courts: courtDTOs})
}

type CreateCourtRequest struct {
	Name string `json:"name" validate:"required,min=1,max=40"`
}

func (h *Handlers) CreateCourt(c *fiber.Ctx) error {
	id, err := ParseUUIDParam(c, "sessionId")
	if err != nil {
		return HandleError(c, err)
	}
	if err := h.requireHostOfSession(c, id); err != nil {
		return HandleError(c, err)
	}
	var req CreateCourtRequest
	if err := BindAndValidate(c, &req); err != nil {
		return HandleError(c, err)
	}
	court, err := h.Sessions.CreateCourt(c.UserContext(), id, req.Name)
	if err != nil {
		return HandleError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(toCourtDTO(court))
}

type JoinSessionRequest struct {
	JoinCode    string `json:"join_code" validate:"required"`
	DisplayName string `json:"display_name" validate:"required,min=1,max=60"`
	Gender      string `json:"gender" validate:"required,oneof=MALE FEMALE"`
	// DeviceID is optional (v2). See CreateClubRequest.
	DeviceID string `json:"device_id" validate:"omitempty,max=100"`
}

type JoinSessionResponse struct {
	Session       SessionDTO         `json:"session"`
	SessionPlayer SessionPlayerDTO   `json:"session_player"`
	You           PlayerAuthResponse `json:"you"`
}

func (h *Handlers) JoinSession(c *fiber.Ctx) error {
	id, err := ParseUUIDParam(c, "sessionId")
	if err != nil {
		return HandleError(c, err)
	}
	var req JoinSessionRequest
	if err := BindAndValidate(c, &req); err != nil {
		return HandleError(c, err)
	}

	result, err := h.Sessions.JoinSession(c.UserContext(), id, session.JoinSessionInput{
		JoinCode: req.JoinCode, DisplayName: req.DisplayName, Gender: player.Gender(req.Gender),
		DeviceID: req.DeviceID,
	})
	if err != nil {
		return HandleError(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(JoinSessionResponse{
		Session:       toSessionDTO(result.Session),
		SessionPlayer: toSessionPlayerDTO(result.SessionPlayer),
		You: PlayerAuthResponse{
			Player:    toPlayerDTO(result.Player),
			Token:     result.Token,
			Returning: !result.IsNewJoin,
		},
	})
}

type RegisterGuestInput struct {
	DisplayName string `json:"display_name" validate:"required,min=1,max=60"`
	Gender      string `json:"gender" validate:"required,oneof=MALE FEMALE"`
}

type RegisterGuestsRequest struct {
	Guests []RegisterGuestInput `json:"guests" validate:"required,min=1,dive"`
}

// RegisterGuests lets a host add players on behalf of people who didn't join
// via the invite link. Names are matched case-insensitively against existing
// club members so re-adding the same guest reuses their identity.
func (h *Handlers) RegisterGuests(c *fiber.Ctx) error {
	id, err := ParseUUIDParam(c, "sessionId")
	if err != nil {
		return HandleError(c, err)
	}
	if err := h.requireHostOfSession(c, id); err != nil {
		return HandleError(c, err)
	}
	var req RegisterGuestsRequest
	if err := BindAndValidate(c, &req); err != nil {
		return HandleError(c, err)
	}

	guests := make([]session.GuestInput, 0, len(req.Guests))
	for _, g := range req.Guests {
		guests = append(guests, session.GuestInput{DisplayName: g.DisplayName, Gender: player.Gender(g.Gender)})
	}

	players, err := h.Sessions.RegisterGuests(c.UserContext(), session.RegisterGuestsInput{SessionID: id, Guests: guests})
	if err != nil {
		return HandleError(c, err)
	}

	dtos := make([]SessionPlayerDTO, 0, len(players))
	for _, p := range players {
		dtos = append(dtos, toSessionPlayerDTO(p))
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"session_players": dtos})
}

type SetSessionPlayerStatusRequest struct {
	Status string `json:"status" validate:"required,oneof=WAITING BREAK ENDED"`
}

func (h *Handlers) SetSessionPlayerStatus(c *fiber.Ctx) error {
	id, err := ParseUUIDParam(c, "id")
	if err != nil {
		return HandleError(c, err)
	}
	principal, err := Principal(c)
	if err != nil {
		return HandleError(c, err)
	}

	sp, err := h.Sessions.GetSessionPlayer(c.UserContext(), id)
	if err != nil {
		return HandleError(c, err)
	}
	if sp.PlayerID != principal.PlayerID {
		// Hosts may also manage player state (e.g. ending a no-show).
		sess, err := h.Sessions.GetSession(c.UserContext(), sp.SessionID)
		if err != nil {
			return HandleError(c, err)
		}
		if err := h.Clubs.RequireHost(c.UserContext(), sess.ClubID, principal.PlayerID); err != nil {
			return HandleError(c, err)
		}
	}

	var req SetSessionPlayerStatusRequest
	if err := BindAndValidate(c, &req); err != nil {
		return HandleError(c, err)
	}

	updated, err := h.Sessions.SetPlayerStatus(c.UserContext(), id, session.PlayerStatus(req.Status))
	if err != nil {
		return HandleError(c, err)
	}
	return c.JSON(toSessionPlayerDTO(updated))
}
