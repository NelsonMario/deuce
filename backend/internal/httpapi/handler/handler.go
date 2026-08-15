package handler

import (
	"log/slog"

	"deuce/backend/internal/club"
	"deuce/backend/internal/match"
	"deuce/backend/internal/player"
	"deuce/backend/internal/session"
)

// Handlers aggregates the application services used by the HTTP layer. It
// depends only on service interfaces/structs, never on Fiber internals of
// other layers, keeping the dependency direction HTTP -> service -> domain.
type Handlers struct {
	Clubs    *club.Service
	Players  *player.Service
	Sessions *session.Service
	Matches  *match.Service
	Logger   *slog.Logger
}

func New(clubs *club.Service, players *player.Service, sessions *session.Service, matches *match.Service, logger *slog.Logger) *Handlers {
	return &Handlers{Clubs: clubs, Players: players, Sessions: sessions, Matches: matches, Logger: logger}
}
