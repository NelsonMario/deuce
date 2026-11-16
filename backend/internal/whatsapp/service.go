package whatsapp

import (
	"context"
	"log/slog"

	"deuce/backend/internal/match"
	"deuce/backend/internal/session"
)

// Service owns the WhatsApp group adapter. It has no player identity or
// private-message concepts.
type Service struct {
	client     Client
	repo       Repository
	config     ConfigStore
	dispatcher *Dispatcher
	worker     *OutboxWorker
}

func NewService(client Client, repo Repository, config ConfigStore, dispatcher *Dispatcher, sessions session.Repository, matches match.ReadRepository, logger *slog.Logger) *Service {
	client.AddEventHandler(dispatcher.HandleEvent)
	return &Service{client: client, repo: repo, config: config, dispatcher: dispatcher, worker: NewOutboxWorker(client, repo, sessions, matches, logger)}
}

func (s *Service) Connect() error            { return s.client.Connect() }
func (s *Service) Disconnect()               { s.client.Disconnect() }
func (s *Service) Run(ctx context.Context)   { s.worker.Run(ctx) }
func (s *Service) CurrentQR() (string, bool) { return s.client.CurrentQR() }
func (s *Service) JoinedGroups(ctx context.Context) ([]Group, error) {
	return s.client.JoinedGroups(ctx)
}
func (s *Service) IsLoggedIn() bool { return s.client.IsLoggedIn() }
func (s *Service) Status(ctx context.Context) map[string]any {
	pending, _ := s.repo.PendingCount(ctx)
	cfg, _ := s.config.Get(ctx)
	return map[string]any{
		"status": string(s.client.GetStatus()), "is_connected": s.client.IsConnected(), "is_logged_in": s.client.IsLoggedIn(),
		"pending_announcements": pending,
		"club_configured":       cfg.ClubID != nil, "admin_group_configured": cfg.AdminGroup != "", "announce_group_configured": cfg.AnnounceGroup != "",
	}
}
