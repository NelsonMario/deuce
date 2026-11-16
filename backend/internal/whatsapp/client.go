package whatsapp

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

type WhatsmeowClient struct {
	client   *whatsmeow.Client
	logger   *slog.Logger
	statusMu sync.RWMutex
	status   ConnectionStatus
	qrChan   chan string
}

func NewWhatsmeowClient(deviceStore *store.Device, logger *slog.Logger) *WhatsmeowClient {
	waLogger := waLog.Stdout("whatsmeow", "INFO", true)
	cli := whatsmeow.NewClient(deviceStore, waLogger)

	wc := &WhatsmeowClient{
		client: cli,
		logger: logger,
		status: StatusDisconnected,
		qrChan: make(chan string, 10),
	}

	cli.AddEventHandler(wc.handleInternalEvents)
	return wc
}

func (w *WhatsmeowClient) AddEventHandler(handler func(evt interface{})) {
	w.client.AddEventHandler(handler)
}

func (w *WhatsmeowClient) IsConnected() bool {
	return w.client.IsConnected()
}

func (w *WhatsmeowClient) IsLoggedIn() bool {
	return w.client.IsLoggedIn()
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

	if w.client.Store.ID == nil {
		// Device not paired yet: QR code flow needed
		w.setStatus(StatusPairingRequired)
		qrChan, err := w.client.GetQRChannel(context.Background())
		if err != nil {
			w.setStatus(StatusDisconnected)
			return fmt.Errorf("get qr channel: %w", err)
		}

		err = w.client.Connect()
		if err != nil {
			w.setStatus(StatusDisconnected)
			return fmt.Errorf("connect client: %w", err)
		}

		go func() {
			for evt := range qrChan {
				switch evt.Event {
				case "code":
					w.logger.Info("whatsmeow_qr_code_received")
					select {
					case w.qrChan <- evt.Code:
					default:
					}
				case "success":
					w.logger.Info("whatsmeow_qr_pair_success")
					w.setStatus(StatusConnected)
				case "timeout":
					w.logger.Warn("whatsmeow_qr_timeout")
					w.setStatus(StatusPairingRequired)
				}
			}
		}()
		return nil
	}

	// Already paired: connect directly
	if err := w.client.Connect(); err != nil {
		w.setStatus(StatusDisconnected)
		return fmt.Errorf("connect client: %w", err)
	}

	w.setStatus(StatusConnected)
	return nil
}

func (w *WhatsmeowClient) Disconnect() {
	w.client.Disconnect()
	w.setStatus(StatusDisconnected)
}

func (w *WhatsmeowClient) GetQRChannel(ctx context.Context) (<-chan string, error) {
	return w.qrChan, nil
}

func (w *WhatsmeowClient) SendMessage(ctx context.Context, recipient types.JID, text string) error {
	msg := &waE2E.Message{
		Conversation: proto.String(text),
	}
	_, err := w.client.SendMessage(ctx, recipient, msg)
	return err
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
