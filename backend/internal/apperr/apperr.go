// Package apperr defines a stable, transport-agnostic application error type
// so that internal error details (e.g. raw database errors) are never leaked
// to HTTP clients.
package apperr

import (
	"errors"
	"fmt"
	"net/http"
)

type Code string

const (
	CodeValidation           Code = "VALIDATION_ERROR"
	CodeNotFound             Code = "NOT_FOUND"
	CodeUnauthorized         Code = "UNAUTHORIZED"
	CodeForbidden            Code = "FORBIDDEN"
	CodeConflict             Code = "CONFLICT"
	CodeInvalidState         Code = "INVALID_STATE"
	CodePlayerNotEligible    Code = "PLAYER_NOT_ELIGIBLE"
	CodeInsufficientPlayers  Code = "INSUFFICIENT_PLAYERS"
	CodeGenerationInProgress Code = "GENERATION_IN_PROGRESS"
	CodeRateLimited          Code = "RATE_LIMITED"
	CodeInternal             Code = "INTERNAL_ERROR"
)

// Error is the canonical application error. Handlers translate it directly
// into the public JSON error envelope.
type Error struct {
	Code    Code
	Message string
	Status  int
	cause   error
}

func (e *Error) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error { return e.cause }

func New(code Code, status int, message string) *Error {
	return &Error{Code: code, Status: status, Message: message}
}

func Wrap(code Code, status int, message string, cause error) *Error {
	return &Error{Code: code, Status: status, Message: message, cause: cause}
}

func NotFound(resource string) *Error {
	return New(CodeNotFound, http.StatusNotFound, resource+" not found")
}

func Validation(message string) *Error {
	return New(CodeValidation, http.StatusBadRequest, message)
}

func Unauthorized(message string) *Error {
	return New(CodeUnauthorized, http.StatusUnauthorized, message)
}

func Forbidden(message string) *Error {
	return New(CodeForbidden, http.StatusForbidden, message)
}

func Conflict(message string) *Error {
	return New(CodeConflict, http.StatusConflict, message)
}

func InvalidState(message string) *Error {
	return New(CodeInvalidState, http.StatusConflict, message)
}

func PlayerNotEligible(message string) *Error {
	return New(CodePlayerNotEligible, http.StatusConflict, message)
}

func InsufficientPlayers(message string) *Error {
	return New(CodeInsufficientPlayers, http.StatusConflict, message)
}

// GenerationInProgress signals that a match-generation request was rejected
// by the short-lived Redis double-tap guard (internal/lock) because another
// generation attempt for the same session+court is already in flight — a
// fast pre-check ahead of the authoritative Postgres row locking, meant to
// catch an accidental double-click or slow-network retry.
func GenerationInProgress(message string) *Error {
	return New(CodeGenerationInProgress, http.StatusConflict, message)
}

// RateLimited signals that a client exceeded its request budget (see
// internal/ratelimit and middleware.RateLimit). Clients should back off and
// retry later.
func RateLimited(message string) *Error {
	return New(CodeRateLimited, http.StatusTooManyRequests, message)
}

func Internal(cause error) *Error {
	return Wrap(CodeInternal, http.StatusInternalServerError, "internal server error", cause)
}

// As extracts an *Error from err, if present.
func As(err error) (*Error, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}
