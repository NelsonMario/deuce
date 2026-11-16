package whatsapp

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// CommandHandler is deliberately small: it keeps WhatsApp transport details
// separate from the existing Deuce application services.
type CommandHandler interface {
	Handle(ctx context.Context, sender types.JID, command string) (string, error)
}

type Dispatcher struct {
	client  Client
	group   types.JID
	handler CommandHandler
	logger  *slog.Logger
	seenMu  sync.Mutex
	seen    map[string]time.Time
}

func NewDispatcher(client Client, groupJID string, handler CommandHandler, logger *slog.Logger) (*Dispatcher, error) {
	group, err := types.ParseJID(groupJID)
	if err != nil {
		return nil, fmt.Errorf("parse WHATSAPP_GROUP_JID: %w", err)
	}
	return &Dispatcher{client: client, group: group, handler: handler, logger: logger, seen: make(map[string]time.Time)}, nil
}

func (d *Dispatcher) HandleEvent(evt interface{}) {
	m, ok := evt.(*events.Message)
	if !ok || m.Message == nil || m.Info.Chat.ToNonAD() != d.group.ToNonAD() {
		return
	}
	if d.isDuplicate(m.Info.ID) {
		return
	}
	text := strings.TrimSpace(m.Message.GetConversation())
	if text == "" && m.Message.GetExtendedTextMessage() != nil {
		text = strings.TrimSpace(m.Message.GetExtendedTextMessage().GetText())
	}
	if text == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		reply, err := d.handler.Handle(ctx, m.Info.Sender.ToNonAD(), text)
		if err != nil {
			d.logger.Warn("whatsapp_command_failed", "error", err)
			reply = "⚠️ Command could not be completed."
		}
		if reply != "" {
			if err := d.client.SendMessage(ctx, d.group, reply); err != nil {
				d.logger.Warn("whatsapp_reply_failed", "error", err)
			}
		}
	}()
}

func (d *Dispatcher) isDuplicate(id string) bool {
	now := time.Now()
	d.seenMu.Lock()
	defer d.seenMu.Unlock()
	for key, seenAt := range d.seen {
		if now.Sub(seenAt) > 10*time.Minute {
			delete(d.seen, key)
		}
	}
	if _, exists := d.seen[id]; exists {
		return true
	}
	d.seen[id] = now
	return false
}
