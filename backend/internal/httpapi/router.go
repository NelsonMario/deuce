// Package httpapi wires the Fiber application: middleware and routes.
package httpapi

import (
	"log/slog"

	"github.com/gofiber/fiber/v2"

	"deuce/backend/internal/auth"
	"deuce/backend/internal/httpapi/handler"
	"deuce/backend/internal/httpapi/middleware"
	"deuce/backend/internal/player"
	"deuce/backend/internal/ratelimit"
	"deuce/backend/internal/version"
)

type Deps struct {
	Handlers    *handler.Handlers
	Hasher      auth.Hasher
	Players     player.Repository
	Logger      *slog.Logger
	CORSOrigins []string
	// RateLimiter backs middleware.RateLimit. Defaults to ratelimit.NoopLimiter
	// (no limiting) if left unset, e.g. in tests that don't care about it.
	RateLimiter ratelimit.Limiter
}

func NewApp(d Deps) *fiber.App {
	app := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			return handler.HandleError(c, err)
		},
	})

	app.Use(middleware.RequestID())
	app.Use(middleware.Recover())
	app.Use(middleware.CORS(d.CORSOrigins))
	app.Use(middleware.Logging(d.Logger))

	rateLimiter := d.RateLimiter
	if rateLimiter == nil {
		rateLimiter = ratelimit.NoopLimiter{}
	}
	app.Use(middleware.RateLimit(rateLimiter, d.Hasher))

	app.Get("/healthz", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "version": version.Version})
	})

	requireAuth := middleware.RequireAuth(d.Hasher, d.Players)

	v1 := app.Group("/api/v1")

	clubs := v1.Group("/clubs")
	clubs.Post("/", d.Handlers.CreateClub)
	clubs.Post("/:clubId/join", d.Handlers.JoinClub)
	clubs.Get("/:clubId", requireAuth, d.Handlers.GetClub)
	clubs.Get("/:clubId/me", requireAuth, d.Handlers.GetMyRole)
	clubs.Get("/:clubId/members", requireAuth, d.Handlers.ListClubMembers)
	clubs.Get("/:clubId/sessions", requireAuth, d.Handlers.ListClubSessions)
	clubs.Post("/:clubId/members/:playerId/promote", requireAuth, d.Handlers.PromoteMember)

	sessions := v1.Group("/sessions")
	sessions.Post("/", requireAuth, d.Handlers.CreateSession)
	sessions.Post("/:sessionId/join", d.Handlers.JoinSession)
	sessions.Get("/:sessionId", requireAuth, d.Handlers.GetSession)
	sessions.Post("/:sessionId/start", requireAuth, d.Handlers.StartSession)
	sessions.Post("/:sessionId/end", requireAuth, d.Handlers.EndSession)
	sessions.Patch("/:sessionId/assignment-mode", requireAuth, d.Handlers.SetAssignmentMode)
	sessions.Patch("/:sessionId/auto-fill", requireAuth, d.Handlers.SetAutoFillEnabled)
	sessions.Post("/:sessionId/courts", requireAuth, d.Handlers.CreateCourt)
	sessions.Post("/:sessionId/guests", requireAuth, d.Handlers.RegisterGuests)
	sessions.Post("/:sessionId/matches/generate", requireAuth, d.Handlers.GenerateMatch)
	sessions.Post("/:sessionId/matches/manual/recommend", requireAuth, d.Handlers.RecommendManualMatch)
	sessions.Post("/:sessionId/matches/manual/confirm", requireAuth, d.Handlers.ConfirmManualMatch)
	sessions.Get("/:sessionId/matches", requireAuth, d.Handlers.ListSessionMatches)

	sessionPlayers := v1.Group("/session-players")
	sessionPlayers.Patch("/:id/status", requireAuth, d.Handlers.SetSessionPlayerStatus)

	matches := v1.Group("/matches")
	matches.Get("/:matchId", requireAuth, d.Handlers.GetMatch)
	matches.Post("/:matchId/start", requireAuth, d.Handlers.StartMatch)
	matches.Post("/:matchId/finish", requireAuth, d.Handlers.FinishMatch)

	players := v1.Group("/players")
	players.Get("/:playerId", requireAuth, d.Handlers.GetPlayer)
	players.Get("/:playerId/rating", requireAuth, d.Handlers.GetPlayerRating)
	players.Get("/:playerId/matches", requireAuth, d.Handlers.ListPlayerMatches)

	return app
}
