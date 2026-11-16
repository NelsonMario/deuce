package handler

import (
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

// GetQR returns the current bot-account pairing code only to an
// authenticated Deuce user. The code is never logged and expires quickly.
// When no code is ready yet (pairing is starting or restarting), it returns
// WAITING and the caller should retry in a few seconds.
func (h *WhatsAppHandler) GetQR(c *fiber.Ctx) error {
	if h.service == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"status": "DISABLED"})
	}
	if h.service.IsLoggedIn() {
		return c.JSON(fiber.Map{"status": "PAIRED"})
	}
	if code, ok := h.service.CurrentQR(); ok {
		return c.JSON(fiber.Map{"qr": code, "status": "READY"})
	}
	return c.JSON(fiber.Map{"status": "WAITING"})
}
