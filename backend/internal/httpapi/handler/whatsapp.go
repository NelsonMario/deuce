package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"deuce/backend/internal/whatsapp"
)

// WhatsAppHandler exposes only operational setup information. It never
// exposes player links or private-message data because this is a group bot.
type WhatsAppHandler struct{ service *whatsapp.Service }

func NewWhatsAppHandler(service *whatsapp.Service) *WhatsAppHandler {
	return &WhatsAppHandler{service: service}
}

func (h *WhatsAppHandler) GetStatus(c *fiber.Ctx) error {
	if h.service == nil {
		return c.JSON(fiber.Map{"status": "DISABLED"})
	}
	return c.JSON(h.service.Status(c.UserContext()))
}

// GetQR returns a bot-account pairing code only to an authenticated Deuce
// user. The code is never logged and is short lived by WhatsApp itself.
func (h *WhatsAppHandler) GetQR(c *fiber.Ctx) error {
	if h.service == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"status": "DISABLED"})
	}
	ch, err := h.service.GetQRChannel(c.UserContext())
	if err != nil {
		return HandleError(c, err)
	}
	select {
	case code := <-ch:
		return c.JSON(fiber.Map{"qr": code, "status": "READY"})
	case <-time.After(2 * time.Second):
		return c.JSON(fiber.Map{"status": "WAITING_OR_PAIRED"})
	case <-c.Context().Done():
		return c.SendStatus(fiber.StatusRequestTimeout)
	}
}
