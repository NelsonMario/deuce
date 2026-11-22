package handler

import (
	"context"
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"deuce/backend/internal/apperr"
	"deuce/backend/internal/database/db"
	"deuce/backend/internal/match"
)

type MatchDTO struct {
	ID        string   `json:"id"`
	SessionID string   `json:"session_id"`
	CourtID   string   `json:"court_id"`
	Format    string   `json:"format"`
	Status    string   `json:"status"`
	StartedAt *string  `json:"started_at,omitempty"`
	EndedAt   *string  `json:"ended_at,omitempty"`
	ScoreA    *int32   `json:"score_a,omitempty"`
	ScoreB    *int32   `json:"score_b,omitempty"`
	Winner    *string  `json:"winner,omitempty"`
	Players   []string `json:"players,omitempty"`
}

func toMatchDTO(m match.Match) MatchDTO {
	dto := MatchDTO{
		ID: m.ID.String(), SessionID: m.SessionID.String(), CourtID: m.CourtID.String(),
		Format: string(m.Format), Status: string(m.Status), ScoreA: m.ScoreA, ScoreB: m.ScoreB,
	}
	if m.StartedAt != nil {
		v := m.StartedAt.Format(timeFormat)
		dto.StartedAt = &v
	}
	if m.EndedAt != nil {
		v := m.EndedAt.Format(timeFormat)
		dto.EndedAt = &v
	}
	if m.Winner != nil {
		v := string(*m.Winner)
		dto.Winner = &v
	}
	if len(m.Players) > 0 {
		dto.Players = make([]string, 0, len(m.Players))
		for _, pid := range m.Players {
			dto.Players = append(dto.Players, pid.String())
		}
	}
	return dto
}

// requireHostOfMatch resolves match -> session -> club and checks the
// principal is that club's host.
func (h *Handlers) requireHostOfMatch(c *fiber.Ctx, matchID uuid.UUID) (match.Match, error) {
	m, err := h.Matches.GetMatch(c.UserContext(), matchID)
	if err != nil {
		return match.Match{}, err
	}
	principal, err := Principal(c)
	if err != nil {
		return match.Match{}, err
	}
	sess, err := h.Sessions.GetSession(c.UserContext(), m.SessionID)
	if err != nil {
		return match.Match{}, err
	}
	if err := h.Clubs.RequireHost(c.UserContext(), sess.ClubID, principal.PlayerID); err != nil {
		return match.Match{}, err
	}
	return m, nil
}

type GenerateMatchRequest struct {
	CourtID string `json:"court_id" validate:"required,uuid"`
	Format  string `json:"format" validate:"required,oneof=MIXED_DOUBLES MEN_DOUBLES WOMEN_DOUBLES"`
}

func (h *Handlers) GenerateMatch(c *fiber.Ctx) error {
	sessionID, err := ParseUUIDParam(c, "sessionId")
	if err != nil {
		return HandleError(c, err)
	}
	if err := h.requireHostOfSession(c, sessionID); err != nil {
		return HandleError(c, err)
	}
	var req GenerateMatchRequest
	if err := BindAndValidate(c, &req); err != nil {
		return HandleError(c, err)
	}
	courtID, err := uuid.Parse(req.CourtID)
	if err != nil {
		return HandleError(c, err)
	}

	m, err := h.Matches.GenerateAutomatic(c.UserContext(), match.GenerateInput{
		SessionID: sessionID, CourtID: courtID, Format: match.Format(req.Format),
	})
	if err != nil {
		return HandleError(c, err)
	}

	m, err = h.Matches.StartMatch(c.UserContext(), m.ID)
	if err != nil {
		return HandleError(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(toMatchDTO(m))
}

type PreviewAutoRequest struct {
	Format string `json:"format" validate:"required,oneof=MIXED_DOUBLES MEN_DOUBLES WOMEN_DOUBLES"`
}

func (h *Handlers) PreviewAutoMatch(c *fiber.Ctx) error {
	sessionID, err := ParseUUIDParam(c, "sessionId")
	if err != nil {
		return HandleError(c, err)
	}
	if err := h.requireHostOfSession(c, sessionID); err != nil {
		return HandleError(c, err)
	}
	var req PreviewAutoRequest
	if err := BindAndValidate(c, &req); err != nil {
		return HandleError(c, err)
	}
	proposal, err := h.Matches.PreviewAutomatic(c.UserContext(), match.PreviewAutoInput{
		SessionID: sessionID, Format: match.Format(req.Format),
	})
	if err != nil {
		return HandleError(c, err)
	}
	return c.JSON(ProposalDTO{
		TeamA:       [2]string{proposal.TeamA[0].String(), proposal.TeamA[1].String()},
		TeamB:       [2]string{proposal.TeamB[0].String(), proposal.TeamB[1].String()},
		TeamARating: proposal.TeamARating, TeamBRating: proposal.TeamBRating, RatingDiff: proposal.RatingDiff,
	})
}

type RecommendManualRequest struct {
	PlayerIDs [4]string `json:"player_ids" validate:"required,len=4,dive,uuid"`
	Format    string    `json:"format" validate:"required,oneof=MIXED_DOUBLES MEN_DOUBLES WOMEN_DOUBLES"`
}

type ProposalDTO struct {
	TeamA       [2]string `json:"team_a"`
	TeamB       [2]string `json:"team_b"`
	TeamARating float64   `json:"team_a_rating"`
	TeamBRating float64   `json:"team_b_rating"`
	RatingDiff  float64   `json:"rating_diff"`
}

func (h *Handlers) RecommendManualMatch(c *fiber.Ctx) error {
	sessionID, err := ParseUUIDParam(c, "sessionId")
	if err != nil {
		return HandleError(c, err)
	}
	if err := h.requireHostOfSession(c, sessionID); err != nil {
		return HandleError(c, err)
	}
	var req RecommendManualRequest
	if err := BindAndValidate(c, &req); err != nil {
		return HandleError(c, err)
	}

	var playerIDs [4]uuid.UUID
	for i, s := range req.PlayerIDs {
		id, err := uuid.Parse(s)
		if err != nil {
			return HandleError(c, err)
		}
		playerIDs[i] = id
	}

	proposal, err := h.Matches.RecommendManual(c.UserContext(), match.RecommendInput{
		SessionID: sessionID, PlayerIDs: playerIDs, Format: match.Format(req.Format),
	})
	if err != nil {
		return HandleError(c, err)
	}

	return c.JSON(ProposalDTO{
		TeamA:       [2]string{proposal.TeamA[0].String(), proposal.TeamA[1].String()},
		TeamB:       [2]string{proposal.TeamB[0].String(), proposal.TeamB[1].String()},
		TeamARating: proposal.TeamARating, TeamBRating: proposal.TeamBRating, RatingDiff: proposal.RatingDiff,
	})
}

type ConfirmManualRequest struct {
	CourtID string    `json:"court_id" validate:"required,uuid"`
	Format  string    `json:"format" validate:"required,oneof=MIXED_DOUBLES MEN_DOUBLES WOMEN_DOUBLES"`
	TeamA   [2]string `json:"team_a" validate:"required,len=2,dive,uuid"`
	TeamB   [2]string `json:"team_b" validate:"required,len=2,dive,uuid"`
}

func (h *Handlers) ConfirmManualMatch(c *fiber.Ctx) error {
	sessionID, err := ParseUUIDParam(c, "sessionId")
	if err != nil {
		return HandleError(c, err)
	}
	if err := h.requireHostOfSession(c, sessionID); err != nil {
		return HandleError(c, err)
	}
	var req ConfirmManualRequest
	if err := BindAndValidate(c, &req); err != nil {
		return HandleError(c, err)
	}

	courtID, err := uuid.Parse(req.CourtID)
	if err != nil {
		return HandleError(c, err)
	}
	teamA := [2]uuid.UUID{uuid.MustParse(req.TeamA[0]), uuid.MustParse(req.TeamA[1])}
	teamB := [2]uuid.UUID{uuid.MustParse(req.TeamB[0]), uuid.MustParse(req.TeamB[1])}

	m, err := h.Matches.ConfirmManual(c.UserContext(), match.ConfirmManualInput{
		SessionID: sessionID, CourtID: courtID, Format: match.Format(req.Format), TeamA: teamA, TeamB: teamB,
	})
	if err != nil {
		return HandleError(c, err)
	}

	m, err = h.Matches.StartMatch(c.UserContext(), m.ID)
	if err != nil {
		return HandleError(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(toMatchDTO(m))
}

func (h *Handlers) ListSessionMatches(c *fiber.Ctx) error {
	sessionID, err := ParseUUIDParam(c, "sessionId")
	if err != nil {
		return HandleError(c, err)
	}
	matches, err := h.Matches.ListBySession(c.UserContext(), sessionID)
	if err != nil {
		return HandleError(c, err)
	}
	dtos := make([]MatchDTO, 0, len(matches))
	for _, m := range matches {
		dtos = append(dtos, toMatchDTO(m))
	}
	return c.JSON(fiber.Map{"matches": dtos})
}

func (h *Handlers) GetMatch(c *fiber.Ctx) error {
	matchID, err := ParseUUIDParam(c, "matchId")
	if err != nil {
		return HandleError(c, err)
	}
	m, err := h.Matches.GetMatch(c.UserContext(), matchID)
	if err != nil {
		return HandleError(c, err)
	}
	return c.JSON(toMatchDTO(m))
}

func (h *Handlers) StartMatch(c *fiber.Ctx) error {
	matchID, err := ParseUUIDParam(c, "matchId")
	if err != nil {
		return HandleError(c, err)
	}
	if _, err := h.requireHostOfMatch(c, matchID); err != nil {
		return HandleError(c, err)
	}
	m, err := h.Matches.StartMatch(c.UserContext(), matchID)
	if err != nil {
		return HandleError(c, err)
	}
	return c.JSON(toMatchDTO(m))
}

type FinishMatchRequest struct {
	ScoreA int32 `json:"score_a" validate:"min=0"`
	ScoreB int32 `json:"score_b" validate:"min=0"`
}

func (h *Handlers) FinishMatch(c *fiber.Ctx) error {
	matchID, err := ParseUUIDParam(c, "matchId")
	if err != nil {
		return HandleError(c, err)
	}
	if _, err := h.requireHostOfMatch(c, matchID); err != nil {
		return HandleError(c, err)
	}
	var req FinishMatchRequest
	if err := BindAndValidate(c, &req); err != nil {
		return HandleError(c, err)
	}

	m, err := h.Matches.FinishMatch(c.UserContext(), match.FinishInput{
		MatchID: matchID, ScoreA: req.ScoreA, ScoreB: req.ScoreB,
	})
	if err != nil {
		return HandleError(c, err)
	}
	return c.JSON(toMatchDTO(m))
}

type UpdateMatchRosterRequest struct {
	TeamA                 [2]string         `json:"team_a" validate:"required,len=2,dive,uuid"`
	TeamB                 [2]string         `json:"team_b" validate:"required,len=2,dive,uuid"`
	RemovedPlayerStatuses map[string]string `json:"removed_player_statuses,omitempty"`
}

func (h *Handlers) UpdateMatchRoster(c *fiber.Ctx) error {
	matchID, err := ParseUUIDParam(c, "matchId")
	if err != nil {
		return HandleError(c, err)
	}
	if _, err := h.requireHostOfMatch(c, matchID); err != nil {
		return HandleError(c, err)
	}
	var req UpdateMatchRosterRequest
	if err := BindAndValidate(c, &req); err != nil {
		return HandleError(c, err)
	}

	teamA := [2]uuid.UUID{uuid.MustParse(req.TeamA[0]), uuid.MustParse(req.TeamA[1])}
	teamB := [2]uuid.UUID{uuid.MustParse(req.TeamB[0]), uuid.MustParse(req.TeamB[1])}

	removedStatuses := map[uuid.UUID]db.SessionPlayerStatus{}
	for pidStr, statusStr := range req.RemovedPlayerStatuses {
		pid, err := uuid.Parse(pidStr)
		if err != nil {
			return HandleError(c, apperr.Validation("removed_player_statuses keys must be player UUIDs"))
		}
		// PLAYING is backend-controlled; anything outside the enum would
		// otherwise reach Postgres and surface as a 500.
		status := db.SessionPlayerStatus(statusStr)
		switch status {
		case db.SessionPlayerStatusWAITING, db.SessionPlayerStatusBREAK, db.SessionPlayerStatusENDED:
			removedStatuses[pid] = status
		default:
			return HandleError(c, apperr.Validation("removed_player_statuses values must be WAITING, BREAK, or ENDED"))
		}
	}

	m, err := h.Matches.UpdateRoster(c.UserContext(), match.UpdateRosterInput{
		MatchID:               matchID,
		TeamA:                 teamA,
		TeamB:                 teamB,
		RemovedPlayerStatuses: removedStatuses,
	})
	if err != nil {
		return HandleError(c, err)
	}
	return c.JSON(toMatchDTO(m))
}

func (h *Handlers) CancelMatch(c *fiber.Ctx) error {
	matchID, err := ParseUUIDParam(c, "matchId")
	if err != nil {
		return HandleError(c, err)
	}
	if _, err := h.requireHostOfMatch(c, matchID); err != nil {
		return HandleError(c, err)
	}
	if err := h.Matches.CancelMatch(c.UserContext(), matchID); err != nil {
		return HandleError(c, err)
	}
	return c.JSON(fiber.Map{"status": "cancelled", "match_id": matchID.String()})
}

func (h *Handlers) resolveSessionPlayerID(ctx context.Context, sessionID uuid.UUID, val string) (uuid.UUID, error) {
	val = strings.TrimSpace(val)
	if val == "" {
		return uuid.Nil, apperr.Validation("player identifier is required")
	}

	// 1. Try parsing as UUID (can be player_id or session_player_id)
	if id, err := uuid.Parse(val); err == nil {
		players, err := h.Sessions.ListPlayers(ctx, sessionID)
		if err == nil {
			for _, sp := range players {
				if sp.PlayerID != nil && *sp.PlayerID == id {
					return id, nil
				}
				if sp.ID == id && sp.PlayerID != nil {
					return *sp.PlayerID, nil
				}
			}
		}
		return id, nil
	}

	// 2. Try looking up by display name in session
	players, err := h.Sessions.ListPlayers(ctx, sessionID)
	if err != nil {
		return uuid.Nil, err
	}
	for _, sp := range players {
		if sp.PlayerID == nil {
			continue
		}
		p, err := h.Players.GetPlayer(ctx, *sp.PlayerID)
		if err == nil && strings.EqualFold(p.DisplayName, val) {
			return *sp.PlayerID, nil
		}
	}

	return uuid.Nil, apperr.NotFound(fmt.Sprintf("player %q in this session", val))
}

type SetPlayersRequest struct {
	SessionID string `json:"session_id,omitempty"`
	Player1   string `json:"player1,omitempty"`
	Player2   string `json:"player2,omitempty"`
	PlayerID1 string `json:"player_id_1,omitempty"`
	PlayerID2 string `json:"player_id_2,omitempty"`
}

type SetPlayersResponse struct {
	Action  string     `json:"action"`
	Message string     `json:"message"`
	Matches []MatchDTO `json:"matches"`
}

// SetPlayers handles the set/swap player endpoint:
// - court <-> court
// - same court
// - court <-> waiting bench
// - bench <-> bench
func (h *Handlers) SetPlayers(c *fiber.Ctx) error {
	sessionID, err := ParseUUIDParam(c, "sessionId")
	var req SetPlayersRequest
	if err := BindAndValidate(c, &req); err != nil {
		return HandleError(c, err)
	}

	if sessionID == uuid.Nil && req.SessionID != "" {
		sessionID, err = uuid.Parse(req.SessionID)
		if err != nil {
			return HandleError(c, apperr.Validation("session_id must be a valid UUID"))
		}
	}
	if sessionID == uuid.Nil {
		return HandleError(c, apperr.Validation("sessionId parameter or session_id field is required"))
	}

	if err := h.requireHostOfSession(c, sessionID); err != nil {
		return HandleError(c, err)
	}

	p1Val := req.Player1
	if p1Val == "" {
		p1Val = req.PlayerID1
	}
	p2Val := req.Player2
	if p2Val == "" {
		p2Val = req.PlayerID2
	}
	if p1Val == "" || p2Val == "" {
		return HandleError(c, apperr.Validation("both player1 and player2 must be specified"))
	}

	p1ID, err := h.resolveSessionPlayerID(c.UserContext(), sessionID, p1Val)
	if err != nil {
		return HandleError(c, err)
	}
	p2ID, err := h.resolveSessionPlayerID(c.UserContext(), sessionID, p2Val)
	if err != nil {
		return HandleError(c, err)
	}

	res, err := h.Matches.SwapPlayers(c.UserContext(), match.SwapPlayersInput{
		SessionID: sessionID,
		Player1ID: p1ID,
		Player2ID: p2ID,
	})
	if err != nil {
		return HandleError(c, err)
	}

	matchDTOs := make([]MatchDTO, 0, len(res.Matches))
	for _, m := range res.Matches {
		matchDTOs = append(matchDTOs, toMatchDTO(m))
	}

	return c.JSON(SetPlayersResponse{
		Action:  string(res.Action),
		Message: res.Message,
		Matches: matchDTOs,
	})
}

type BatchUpdateRosterItem struct {
	MatchID               string            `json:"match_id" validate:"required,uuid"`
	TeamA                 [2]string         `json:"team_a" validate:"required,len=2,dive,uuid"`
	TeamB                 [2]string         `json:"team_b" validate:"required,len=2,dive,uuid"`
	RemovedPlayerStatuses map[string]string `json:"removed_player_statuses,omitempty"`
}

type BatchUpdateRostersRequest struct {
	Updates []BatchUpdateRosterItem `json:"updates" validate:"required,min=1"`
}

func (h *Handlers) BatchUpdateRosters(c *fiber.Ctx) error {
	var req BatchUpdateRostersRequest
	if err := BindAndValidate(c, &req); err != nil {
		return HandleError(c, err)
	}

	inputs := make([]match.UpdateRosterInput, 0, len(req.Updates))
	for _, item := range req.Updates {
		mID := uuid.MustParse(item.MatchID)
		if _, err := h.requireHostOfMatch(c, mID); err != nil {
			return HandleError(c, err)
		}
		teamA := [2]uuid.UUID{uuid.MustParse(item.TeamA[0]), uuid.MustParse(item.TeamA[1])}
		teamB := [2]uuid.UUID{uuid.MustParse(item.TeamB[0]), uuid.MustParse(item.TeamB[1])}

		removedStatuses := map[uuid.UUID]db.SessionPlayerStatus{}
		for pidStr, statusStr := range item.RemovedPlayerStatuses {
			pid, err := uuid.Parse(pidStr)
			if err != nil {
				return HandleError(c, apperr.Validation("removed_player_statuses keys must be player UUIDs"))
			}
			status := db.SessionPlayerStatus(statusStr)
			switch status {
			case db.SessionPlayerStatusWAITING, db.SessionPlayerStatusBREAK, db.SessionPlayerStatusENDED:
				removedStatuses[pid] = status
			default:
				return HandleError(c, apperr.Validation("removed_player_statuses values must be WAITING, BREAK, or ENDED"))
			}
		}

		inputs = append(inputs, match.UpdateRosterInput{
			MatchID:               mID,
			TeamA:                 teamA,
			TeamB:                 teamB,
			RemovedPlayerStatuses: removedStatuses,
		})
	}

	results, err := h.Matches.UpdateRosters(c.UserContext(), inputs)
	if err != nil {
		return HandleError(c, err)
	}

	dtos := make([]MatchDTO, 0, len(results))
	for _, m := range results {
		dtos = append(dtos, toMatchDTO(m))
	}
	return c.JSON(dtos)
}
