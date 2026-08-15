package match_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"

	"deuce/backend/internal/apperr"
	"deuce/backend/internal/match"
)

// alwaysLockedLocker simulates another in-flight request already holding
// the double-tap guard: every TryAcquire fails, regardless of key.
type alwaysLockedLocker struct{}

func (alwaysLockedLocker) TryAcquire(context.Context, string) bool { return false }
func (alwaysLockedLocker) Release(context.Context, string)         {}

// These are unit tests (no Postgres needed): TryAcquire is checked, and the
// lock rejection short-circuits before either method touches the DB pool,
// so a nil pool/read-repository is safe here.
func newLockedService() *match.Service {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return match.NewService(nil, nil, alwaysLockedLocker{}, logger)
}

func TestGenerateAutomatic_FailsFastWhenLockHeld(t *testing.T) {
	s := newLockedService()

	_, err := s.GenerateAutomatic(context.Background(), match.GenerateInput{
		SessionID: uuid.New(),
		CourtID:   uuid.New(),
		Format:    match.MenDoubles,
	})
	if err == nil {
		t.Fatal("expected an error when the double-tap lock is already held")
	}
	appErr, ok := apperr.As(err)
	if !ok {
		t.Fatalf("expected an *apperr.Error, got %T: %v", err, err)
	}
	if appErr.Code != apperr.CodeGenerationInProgress {
		t.Fatalf("expected code %s, got %s", apperr.CodeGenerationInProgress, appErr.Code)
	}
}

func TestConfirmManual_FailsFastWhenLockHeld(t *testing.T) {
	s := newLockedService()

	_, err := s.ConfirmManual(context.Background(), match.ConfirmManualInput{
		SessionID: uuid.New(),
		CourtID:   uuid.New(),
		Format:    match.MenDoubles,
		TeamA:     [2]uuid.UUID{uuid.New(), uuid.New()},
		TeamB:     [2]uuid.UUID{uuid.New(), uuid.New()},
	})
	if err == nil {
		t.Fatal("expected an error when the double-tap lock is already held")
	}
	appErr, ok := apperr.As(err)
	if !ok {
		t.Fatalf("expected an *apperr.Error, got %T: %v", err, err)
	}
	if appErr.Code != apperr.CodeGenerationInProgress {
		t.Fatalf("expected code %s, got %s", apperr.CodeGenerationInProgress, appErr.Code)
	}
}
