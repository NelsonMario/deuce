// Package handler implements the Fiber HTTP handlers. It is the only layer
// that knows about Fiber; it translates requests into application-service
// calls and translates apperr.Error into the public JSON error envelope.
package handler

import (
	"errors"
	"net/http"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"deuce/backend/internal/apperr"
	"deuce/backend/internal/auth"
)

var validate = validator.New()

// ErrorEnvelope is the stable public error shape (spec section 24).
type ErrorEnvelope struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// HandleError converts any error into the appropriate HTTP response. It
// never leaks raw database/internal error strings to the client.
func HandleError(c *fiber.Ctx, err error) error {
	if appErr, ok := apperr.As(err); ok {
		return c.Status(appErr.Status).JSON(ErrorEnvelope{
			Error: ErrorBody{Code: string(appErr.Code), Message: appErr.Message},
		})
	}
	return c.Status(http.StatusInternalServerError).JSON(ErrorEnvelope{
		Error: ErrorBody{Code: string(apperr.CodeInternal), Message: "internal server error"},
	})
}

// BindAndValidate parses the JSON body into dst and runs struct validation
// tags (go-playground/validator) at the HTTP boundary.
func BindAndValidate(c *fiber.Ctx, dst any) error {
	if err := c.BodyParser(dst); err != nil {
		return apperr.Validation("invalid request body")
	}
	if err := validate.Struct(dst); err != nil {
		var ve validator.ValidationErrors
		if errors.As(err, &ve) && len(ve) > 0 {
			return apperr.Validation(ve[0].Field() + " " + ve[0].Tag())
		}
		return apperr.Validation("validation failed")
	}
	return nil
}

// Principal returns the authenticated caller, set by middleware.RequireAuth.
func Principal(c *fiber.Ctx) (auth.Principal, error) {
	p, ok := c.Locals("principal").(auth.Principal)
	if !ok {
		return auth.Principal{}, apperr.Unauthorized("authentication required")
	}
	return p, nil
}

// ParseUUIDParam parses a route param into a uuid, returning a validation
// apperr on failure.
func ParseUUIDParam(c *fiber.Ctx, name string) (uuid.UUID, error) {
	id, err := uuid.Parse(c.Params(name))
	if err != nil {
		return uuid.UUID{}, apperr.Validation("invalid " + name)
	}
	return id, nil
}
