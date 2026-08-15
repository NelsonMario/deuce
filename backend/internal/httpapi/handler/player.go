package handler

import (
	"strconv"

	"github.com/gofiber/fiber/v2"

	"deuce/backend/internal/apperr"
	"deuce/backend/internal/player"
)

type PlayerDTO struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Gender      string `json:"gender"`
	IsGuest     bool   `json:"is_guest"`
}

func toPlayerDTO(p player.Player) PlayerDTO {
	return PlayerDTO{ID: p.ID.String(), DisplayName: p.DisplayName, Gender: string(p.Gender), IsGuest: p.IsGuest}
}

func (h *Handlers) GetPlayer(c *fiber.Ctx) error {
	id, err := ParseUUIDParam(c, "playerId")
	if err != nil {
		return HandleError(c, err)
	}
	p, err := h.Players.GetPlayer(c.UserContext(), id)
	if err != nil {
		return HandleError(c, err)
	}
	return c.JSON(toPlayerDTO(p))
}

type RatingDTO struct {
	PlayerID string  `json:"player_id"`
	Rating   float64 `json:"rating"`
}

func (h *Handlers) GetPlayerRating(c *fiber.Ctx) error {
	id, err := ParseUUIDParam(c, "playerId")
	if err != nil {
		return HandleError(c, err)
	}
	r, err := h.Players.GetRating(c.UserContext(), id)
	if err != nil {
		return HandleError(c, err)
	}
	return c.JSON(RatingDTO{PlayerID: r.PlayerID.String(), Rating: r.Rating})
}

type MatchSummaryDTO struct {
	MatchID   string  `json:"match_id"`
	SessionID string  `json:"session_id"`
	Format    string  `json:"format"`
	Status    string  `json:"status"`
	ScoreA    *int32  `json:"score_a,omitempty"`
	ScoreB    *int32  `json:"score_b,omitempty"`
	Winner    *string `json:"winner,omitempty"`
}

func (h *Handlers) ListPlayerMatches(c *fiber.Ctx) error {
	id, err := ParseUUIDParam(c, "playerId")
	if err != nil {
		return HandleError(c, err)
	}
	limit, offset := pagination(c)
	matches, err := h.Players.ListMatches(c.UserContext(), id, limit, offset)
	if err != nil {
		return HandleError(c, err)
	}
	dtos := make([]MatchSummaryDTO, 0, len(matches))
	for _, m := range matches {
		dtos = append(dtos, MatchSummaryDTO{
			MatchID: m.MatchID.String(), SessionID: m.SessionID.String(),
			Format: m.Format, Status: m.Status, ScoreA: m.ScoreA, ScoreB: m.ScoreB, Winner: m.Winner,
		})
	}
	return c.JSON(fiber.Map{"matches": dtos})
}

type CleanupGuestsRequest struct {
	RetentionDays int `json:"retention_days"`
}

type CleanupGuestsResponse struct {
	Deleted int `json:"deleted"`
}

// CleanupGuests deletes guest players not seen in RetentionDays days,
// skipping guests still in a not-started/active session. It's gated by
// middleware.RequireCronSecret, not player auth — see the guest cleanup
// GitHub Actions workflow, which calls this on a schedule instead of
// running SQL against the database directly.
func (h *Handlers) CleanupGuests(c *fiber.Ctx) error {
	req := CleanupGuestsRequest{RetentionDays: 30}
	if len(c.Body()) > 0 {
		if err := c.BodyParser(&req); err != nil {
			return HandleError(c, apperr.Validation("invalid request body"))
		}
	}

	deleted, err := h.Players.CleanupStaleGuests(c.UserContext(), req.RetentionDays)
	if err != nil {
		return HandleError(c, err)
	}
	return c.JSON(CleanupGuestsResponse{Deleted: deleted})
}

func pagination(c *fiber.Ctx) (int32, int32) {
	limit, err := strconv.Atoi(c.Query("limit", "20"))
	if err != nil || limit <= 0 || limit > 100 {
		limit = 20
	}
	offset, err := strconv.Atoi(c.Query("offset", "0"))
	if err != nil || offset < 0 {
		offset = 0
	}
	return int32(limit), int32(offset)
}
