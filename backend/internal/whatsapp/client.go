package whatsapp

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

// qrValidity is how long a pairing code is offered after WhatsApp issued it.
// WhatsApp rotates codes roughly every 20s (60s for the first one), so an
// older code has almost certainly been replaced.
const qrValidity = 60 * time.Second

type WhatsmeowClient struct {
	// container creates a fresh device after the bot is unlinked: whatsmeow
	// marks the old device as deleted and refuses to connect with it again.
	container DeviceContainer

	// clientMu guards client and handlers; the client is replaced on unlink.
	clientMu sync.RWMutex
	client   *whatsmeow.Client
	handlers []func(evt interface{})

	logger   *slog.Logger
	statusMu sync.RWMutex
	status   ConnectionStatus

	// qrMu guards the pairing state. Only the latest code is kept: a queue
	// of codes would hand out expired ones.
	qrMu    sync.Mutex
	qrCode  string
	qrAt    time.Time
	pairing bool
}

// DeviceContainer creates new whatsmeow devices (sqlstore.Container).
type DeviceContainer interface {
	NewDevice() *store.Device
}

func NewWhatsmeowClient(container DeviceContainer, deviceStore *store.Device, logger *slog.Logger) *WhatsmeowClient {
	wc := &WhatsmeowClient{
		container: container,
		logger:    logger,
		status:    StatusDisconnected,
	}
	wc.client = wc.newClient(deviceStore)
	return wc
}

func (w *WhatsmeowClient) newClient(device *store.Device) *whatsmeow.Client {
	cli := whatsmeow.NewClient(device, waLog.Stdout("whatsmeow", "INFO", true))
	cli.AddEventHandler(w.handleInternalEvents)
	for _, h := range w.handlers {
		cli.AddEventHandler(h)
	}
	return cli
}

func (w *WhatsmeowClient) cli() *whatsmeow.Client {
	w.clientMu.RLock()
	defer w.clientMu.RUnlock()
	return w.client
}

// unpaired reports whether the bot has no usable login: never paired, or
// unlinked from the phone (whatsmeow then deletes the device for good).
func (w *WhatsmeowClient) unpaired() bool {
	st := w.cli().Store
	return st.ID == nil || st.Deleted
}

// resetDevice swaps in a client with a fresh device so pairing can start
// again after the bot was unlinked, without restarting the backend.
func (w *WhatsmeowClient) resetDevice() {
	w.clientMu.Lock()
	old := w.client
	w.client = w.newClient(w.container.NewDevice())
	w.clientMu.Unlock()
	old.Disconnect()
	w.logger.Info("whatsmeow_device_reset")
}

func (w *WhatsmeowClient) AddEventHandler(handler func(evt interface{})) {
	w.clientMu.Lock()
	w.handlers = append(w.handlers, handler)
	cli := w.client
	w.clientMu.Unlock()
	cli.AddEventHandler(handler)
}

func (w *WhatsmeowClient) IsConnected() bool {
	return w.cli().IsConnected()
}

func (w *WhatsmeowClient) IsLoggedIn() bool {
	return w.cli().IsLoggedIn()
}

func (w *WhatsmeowClient) GetStatus() ConnectionStatus {
	w.statusMu.RLock()
	defer w.statusMu.RUnlock()
	return w.status
}

func (w *WhatsmeowClient) setStatus(s ConnectionStatus) {
	w.statusMu.Lock()
	w.status = s
	w.statusMu.Unlock()
}

func (w *WhatsmeowClient) Connect() error {
	w.setStatus(StatusConnecting)

	if w.unpaired() {
		return w.startPairing()
	}

	// Already paired: connect directly. whatsmeow reconnects by itself after
	// a connection has been established once.
	if err := w.cli().Connect(); err != nil {
		w.setStatus(StatusDisconnected)
		return fmt.Errorf("connect client: %w", err)
	}

	w.setStatus(StatusConnected)
	return nil
}

// startPairing opens a QR pairing flow for an unpaired device. When the flow
// times out (nobody scanned), the connection is closed and a later
// CurrentQR call starts a new flow, so pairing never needs a restart.
func (w *WhatsmeowClient) startPairing() error {
	w.qrMu.Lock()
	if w.pairing {
		w.qrMu.Unlock()
		return nil
	}
	w.pairing = true
	w.qrMu.Unlock()

	done := func() {
		w.qrMu.Lock()
		w.pairing = false
		w.qrCode = ""
		w.qrMu.Unlock()
	}

	if w.cli().Store.Deleted {
		w.resetDevice()
	}
	cli := w.cli()

	w.setStatus(StatusPairingRequired)
	qrChan, err := cli.GetQRChannel(context.Background())
	if err != nil {
		done()
		w.setStatus(StatusDisconnected)
		return fmt.Errorf("get qr channel: %w", err)
	}
	if err := cli.Connect(); err != nil {
		done()
		w.setStatus(StatusDisconnected)
		return fmt.Errorf("connect client: %w", err)
	}

	go func() {
		defer done()
		for evt := range qrChan {
			switch evt.Event {
			case "code":
				w.logger.Info("whatsmeow_qr_code_received")
				w.qrMu.Lock()
				w.qrCode, w.qrAt = evt.Code, time.Now()
				w.qrMu.Unlock()
			case "success":
				w.logger.Info("whatsmeow_qr_pair_success")
				w.setStatus(StatusConnected)
			default:
				// "timeout" or a pairing error: the channel closes next.
				w.logger.Warn("whatsmeow_qr_pairing_ended", "event", evt.Event)
				w.setStatus(StatusPairingRequired)
				cli.Disconnect()
			}
		}
	}()
	return nil
}

func (w *WhatsmeowClient) Disconnect() {
	w.cli().Disconnect()
	w.setStatus(StatusDisconnected)
}

// CurrentQR returns the latest unexpired pairing code. If the device is not
// paired and no pairing flow is running (e.g. the previous one timed out),
// it starts a new flow; the code is then available on the next call.
func (w *WhatsmeowClient) CurrentQR() (string, bool) {
	w.qrMu.Lock()
	code, at, pairing := w.qrCode, w.qrAt, w.pairing
	w.qrMu.Unlock()

	if code != "" && time.Since(at) < qrValidity {
		return code, true
	}
	if w.unpaired() && !pairing {
		go func() {
			if err := w.startPairing(); err != nil {
				w.logger.Warn("whatsmeow_pairing_restart_failed", "error", err)
			}
		}()
	}
	return "", false
}

func (w *WhatsmeowClient) SendMessage(ctx context.Context, recipient types.JID, text string) error {
	msg := &waE2E.Message{
		Conversation: proto.String(text),
	}
	_, err := w.cli().SendMessage(ctx, recipient, msg)
	return err
}

func (w *WhatsmeowClient) JoinedGroups(ctx context.Context) ([]Group, error) {
	groups, err := w.cli().GetJoinedGroups(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Group, 0, len(groups))
	for _, g := range groups {
		out = append(out, Group{JID: g.JID.String(), Name: g.GroupName.Name})
	}
	return out, nil
}

func (w *WhatsmeowClient) handleInternalEvents(evt interface{}) {
	switch v := evt.(type) {
	case *events.Connected:
		w.logger.Info("whatsmeow_event_connected")
		w.setStatus(StatusConnected)
	case *events.LoggedOut:
		w.logger.Warn("whatsmeow_event_logged_out", "reason", v.Reason)
		w.setStatus(StatusPairingRequired)
	case *events.Disconnected:
		w.logger.Warn("whatsmeow_event_disconnected")
		w.setStatus(StatusDisconnected)
	case *events.StreamReplaced:
		w.logger.Warn("whatsmeow_event_stream_replaced")
		w.setStatus(StatusDisconnected)
	}
}
