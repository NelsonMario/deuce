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

// CommandHandler runs an authorized command and returns the reply.
type CommandHandler interface {
	Handle(ctx context.Context, req Request) (string, error)
}

// maxCommandAge drops messages older than this. After a reconnect WhatsApp
// replays the offline backlog; re-running an old `next` or `stop` would act
// on a session state the admin never saw.
const maxCommandAge = 2 * time.Minute

// Dispatcher routes group messages:
//   - admin group: every message is a potential command (all members are
//     admins, since only members can post there);
//   - announcement group: ignored, so player chatter never triggers the bot;
//   - `setup` typed on the bot's own phone: accepted in any group;
//   - legacy single-group mode (no admin group set up yet): commands in the
//     announcement group from WHATSAPP_ADMIN_JIDS, as before `setup` existed.
//
// Private chats are always ignored.
type Dispatcher struct {
	client       Client
	config       ConfigStore
	handler      CommandHandler
	legacyAdmins map[string]struct{}
	logger       *slog.Logger
	seenMu       sync.Mutex
	seen         map[string]time.Time
}

func NewDispatcher(client Client, config ConfigStore, legacyAdminJIDs []string, handler CommandHandler, logger *slog.Logger) *Dispatcher {
	admins := make(map[string]struct{}, len(legacyAdminJIDs))
	for _, jid := range legacyAdminJIDs {
		if jid = strings.TrimSpace(jid); jid != "" {
			admins[jid] = struct{}{}
		}
	}
	return &Dispatcher{client: client, config: config, handler: handler, legacyAdmins: admins, logger: logger, seen: make(map[string]time.Time)}
}

func (d *Dispatcher) HandleEvent(evt interface{}) {
	m, ok := evt.(*events.Message)
	if !ok || m.Message == nil || m.Info.Chat.Server != types.GroupServer {
		return
	}
	text := strings.TrimSpace(m.Message.GetConversation())
	if text == "" && m.Message.GetExtendedTextMessage() != nil {
		text = strings.TrimSpace(m.Message.GetExtendedTextMessage().GetText())
	}
	cmd, isCommand := ParseCommand(text)
	if !isCommand {
		return
	}
	// Message text is never logged; only routing metadata, so an operator
	// can see why a command did or didn't get a reply.
	ignore := func(reason string) {
		d.logger.Info("whatsapp_command_ignored", "reason", reason, "chat", m.Info.Chat.String(),
			"sender", m.Info.Sender.String(), "sender_alt", m.Info.SenderAlt.String(), "from_me", m.Info.IsFromMe)
	}
	if !m.Info.Timestamp.IsZero() && time.Since(m.Info.Timestamp) > maxCommandAge {
		ignore("too_old")
		return
	}
	if d.isDuplicate(m.Info.ID) {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	cfg, err := d.config.Get(ctx)
	cancel()
	if err != nil {
		d.logger.Warn("whatsapp_config_load_failed", "error", err)
		return
	}
	chat := m.Info.Chat.ToNonAD().String()
	switch {
	case cmd.Name == "setup":
		// GroupCommands checks that it was typed on the bot's phone.
	case cfg.AdminGroup != "" && chat == cfg.AdminGroup:
	case cfg.AdminGroup == "" && chat == cfg.AnnounceGroup && (m.Info.IsFromMe || d.isLegacyAdmin(m.Info.Sender, m.Info.SenderAlt)):
	case chat == cfg.AnnounceGroup:
		ignore("announcement_group")
		return
	default:
		ignore("unconfigured_group")
		return
	}

	d.logger.Info("whatsapp_command_received", "command", cmd.Name, "chat", chat, "sender", m.Info.Sender.String(), "from_me", m.Info.IsFromMe)
	req := Request{Chat: m.Info.Chat.ToNonAD(), FromMe: m.Info.IsFromMe, Text: text}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		reply, err := d.handler.Handle(ctx, req)
		if err != nil {
			d.logger.Warn("whatsapp_command_failed", "command", cmd.Name, "error", err)
			reply = "⚠️ Command could not be completed."
		}
		if reply == "" {
			return
		}
		if err := d.client.SendMessage(ctx, req.Chat, reply); err != nil {
			d.logger.Warn("whatsapp_reply_failed", "error", err)
		} else {
			d.logger.Info("whatsapp_reply_sent", "command", cmd.Name)
		}
	}()
}

// isLegacyAdmin checks WHATSAPP_ADMIN_JIDS against both sender addresses
// (groups may address members by LID, with the phone number as the alt).
func (d *Dispatcher) isLegacyAdmin(jids ...types.JID) bool {
	for _, jid := range jids {
		if jid.IsEmpty() {
			continue
		}
		if _, ok := d.legacyAdmins[jid.ToNonAD().String()]; ok {
			return true
		}
	}
	return false
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

// GroupAnnouncer posts to the configured announcement group.
type GroupAnnouncer struct {
	client Client
	config ConfigStore
}

func NewGroupAnnouncer(client Client, config ConfigStore) *GroupAnnouncer {
	return &GroupAnnouncer{client: client, config: config}
}

func (a *GroupAnnouncer) Announce(ctx context.Context, text string) error {
	cfg, err := a.config.Get(ctx)
	if err != nil || cfg.AnnounceGroup == "" {
		return err
	}
	jid, err := types.ParseJID(cfg.AnnounceGroup)
	if err != nil {
		return fmt.Errorf("parse announcement group: %w", err)
	}
	return a.client.SendMessage(ctx, jid, text)
}
