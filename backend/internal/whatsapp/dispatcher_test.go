package whatsapp

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

type staticConfig struct{ cfg Config }

func (s *staticConfig) Get(context.Context) (Config, error) { return s.cfg, nil }
func (s *staticConfig) SetAdminGroup(context.Context, string, uuid.UUID) error {
	return nil
}
func (s *staticConfig) SetAnnounceGroup(context.Context, string) error { return nil }

type recordingHandler struct{ got chan Request }

func (h *recordingHandler) Handle(_ context.Context, req Request) (string, error) {
	h.got <- req
	return "", nil
}

type nopClient struct{}

func (nopClient) Connect() error                                       { return nil }
func (nopClient) Disconnect()                                          {}
func (nopClient) IsConnected() bool                                    { return true }
func (nopClient) IsLoggedIn() bool                                     { return true }
func (nopClient) GetStatus() ConnectionStatus                          { return StatusConnected }
func (nopClient) CurrentQR() (string, bool)                            { return "", false }
func (nopClient) SendMessage(context.Context, types.JID, string) error { return nil }
func (nopClient) AddEventHandler(func(interface{}))                    {}
func (nopClient) JoinedGroups(context.Context) ([]Group, error)        { return nil, nil }

const (
	tAdmin    = "120363000000000001@g.us"
	tAnnounce = "120363000000000002@g.us"
	tOther    = "120363000000000003@g.us"
	tLegacy   = "628111111111@s.whatsapp.net"
)

func msg(t *testing.T, chat, sender string, fromMe bool, text string) *events.Message {
	t.Helper()
	c, _ := types.ParseJID(chat)
	s, _ := types.ParseJID(sender)
	return &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{Chat: c, Sender: s, IsFromMe: fromMe, IsGroup: c.Server == types.GroupServer},
			ID:            uuid.NewString(),
			Timestamp:     time.Now(),
		},
		Message: &waE2E.Message{Conversation: proto.String(text)},
	}
}

func TestDispatcherRouting(t *testing.T) {
	split := Config{AdminGroup: tAdmin, AnnounceGroup: tAnnounce}
	legacy := Config{AnnounceGroup: tAnnounce}
	member := "628999999999@s.whatsapp.net"

	cases := []struct {
		name    string
		cfg     Config
		evt     *events.Message
		handled bool
	}{
		{"admin group command", split, msg(t, tAdmin, member, false, "status"), true},
		{"admin group chatter", split, msg(t, tAdmin, member, false, "on my way"), false},
		{"announce group command is ignored", split, msg(t, tAnnounce, tLegacy, false, "next"), false},
		{"unconfigured group", split, msg(t, tOther, member, false, "status"), false},
		{"private chat", split, msg(t, member, member, false, "status"), false},
		{"setup from bot phone anywhere", split, msg(t, tOther, member, true, "setup admin"), true},
		{"legacy admin in single group", legacy, msg(t, tAnnounce, tLegacy, false, "next"), true},
		{"legacy non-admin in single group", legacy, msg(t, tAnnounce, member, false, "next"), false},
		{"too old", split, func() *events.Message {
			m := msg(t, tAdmin, member, false, "status")
			m.Info.Timestamp = time.Now().Add(-10 * time.Minute)
			return m
		}(), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := &recordingHandler{got: make(chan Request, 1)}
			d := NewDispatcher(nopClient{}, &staticConfig{cfg: tc.cfg}, []string{tLegacy}, h, slog.New(slog.NewTextHandler(io.Discard, nil)))
			d.HandleEvent(tc.evt)
			select {
			case req := <-h.got:
				if !tc.handled {
					t.Fatalf("expected message to be ignored, got %+v", req)
				}
				if req.Chat.String() != tc.evt.Info.Chat.String() {
					t.Fatalf("reply chat %s, want %s", req.Chat, tc.evt.Info.Chat)
				}
			case <-time.After(200 * time.Millisecond):
				if tc.handled {
					t.Fatal("expected message to be handled")
				}
			}
		})
	}
}
