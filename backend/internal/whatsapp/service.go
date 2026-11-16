package whatsapp

import (
	"context"
	"log/slog"

	"deuce/backend/internal/match"
	"deuce/backend/internal/session"
)

// Service owns the one-group WhatsApp adapter. It has no player identity or
// private-message concepts.
type Service struct {
	client     Client
	repo       Repository
	dispatcher *Dispatcher
	worker     *OutboxWorker
}

func NewService(client Client, repo Repository, dispatcher *Dispatcher, sessions session.Repository, matches match.ReadRepository, logger *slog.Logger) *Service {
	client.AddEventHandler(dispatcher.HandleEvent)
	return &Service{client: client, repo: repo, dispatcher: dispatcher, worker: NewOutboxWorker(client, repo, sessions, matches, logger)}
}

func (s *Service) Connect() error          { return s.client.Connect() }
func (s *Service) Disconnect()             { s.client.Disconnect() }
func (s *Service) Run(ctx context.Context) { s.worker.Run(ctx) }
func (s *Service) GetQRChannel(ctx context.Context) (<-chan string, error) {
	return s.client.GetQRChannel(ctx)
}
func (s *Service) Status(ctx context.Context) map[string]any {
	pending, _ := s.repo.PendingCount(ctx)
	return map[string]any{"status": string(s.client.GetStatus()), "is_connected": s.client.IsConnected(), "is_logged_in": s.client.IsLoggedIn(), "pending_announcements": pending}
}
