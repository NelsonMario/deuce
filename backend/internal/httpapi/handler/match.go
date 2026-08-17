package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

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
