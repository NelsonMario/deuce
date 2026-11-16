package whatsapp

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.mau.fi/whatsmeow/types"
)

type ConnectionStatus string

const (
	StatusDisconnected    ConnectionStatus = "DISCONNECTED"
	StatusConnecting      ConnectionStatus = "CONNECTING"
	StatusConnected       ConnectionStatus = "CONNECTED"
	StatusPairingRequired ConnectionStatus = "PAIRING_REQUIRED"
)

type OutboxStatus string

const (
	OutboxPending   OutboxStatus = "PENDING"
	OutboxSending   OutboxStatus = "SENDING"
	OutboxSent      OutboxStatus = "SENT"
	OutboxFailed    OutboxStatus = "FAILED"
	OutboxCancelled OutboxStatus = "CANCELLED"
)

// Announcement is intentionally public: it is sent only to the configured
// badminton group and contains no phone numbers or player credentials.
type Announcement struct {
	CourtName string   `json:"court_name"`
	Format    string   `json:"format"`
	TeamA     []string `json:"team_a"`
	TeamB     []string `json:"team_b"`
}

type OutboxItem struct {
	ID          uuid.UUID
	SessionID   uuid.UUID
	MatchID     uuid.UUID
	GroupJID    string
	Payload     Announcement
	Status      OutboxStatus
	Attempts    int
	NextAttempt time.Time
	CreatedAt   time.Time
}

// Client is the small whatsmeow surface needed by this package.
type Client interface {
	Connect() error
	Disconnect()
	IsConnected() bool
	IsLoggedIn() bool
	GetStatus() ConnectionStatus
	// CurrentQR returns the latest unexpired pairing code, if any.
	CurrentQR() (code string, ok bool)
	SendMessage(ctx context.Context, recipient types.JID, text string) error
	AddEventHandler(handler func(evt interface{}))
}
