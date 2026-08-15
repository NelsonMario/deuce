package session

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"deuce/backend/internal/database/db"
)

var ErrNotFound = errors.New("session: not found")

type Repository interface {
	CreateSession(ctx context.Context, clubID uuid.UUID, name string, mode AssignmentMode) (Session, error)
	GetByID(ctx context.Context, id uuid.UUID) (Session, error)
	StartSession(ctx context.Context, id uuid.UUID) (Session, error)
	EndSession(ctx context.Context, id uuid.UUID) (Session, error)
	UpdateAssignmentMode(ctx context.Context, id uuid.UUID, mode AssignmentMode) (Session, error)
	UpdateAutoFillEnabled(ctx context.Context, id uuid.UUID, enabled bool) (Session, error)
	ListByClub(ctx context.Context, clubID uuid.UUID) ([]Session, error)
	// ListActiveAutoFillSessions returns every ACTIVE, AUTOMATIC-assignment
	// session with auto_fill_enabled set — the working set polled by the
	// background auto-fill job (see internal/match/autofill.go).
	ListActiveAutoFillSessions(ctx context.Context) ([]Session, error)

	AddSessionPlayer(ctx context.Context, sessionID, playerID uuid.UUID) (Player, error)
	GetSessionPlayer(ctx context.Context, id uuid.UUID) (Player, error)
	GetSessionPlayerBySessionAndPlayer(ctx context.Context, sessionID, playerID uuid.UUID) (Player, error)
	ListSessionPlayers(ctx context.Context, sessionID uuid.UUID) ([]Player, error)
	SetSessionPlayerStatus(ctx context.Context, id uuid.UUID, status PlayerStatus) (Player, error)

	CreateCourt(ctx context.Context, sessionID uuid.UUID, name string) (Court, error)
	ListCourts(ctx context.Context, sessionID uuid.UUID) ([]Court, error)
	// ListAvailableCourts returns only AVAILABLE courts for a session — used
	// by the auto-fill job to find courts it can generate a match for.
	ListAvailableCourts(ctx context.Context, sessionID uuid.UUID) ([]Court, error)
	GetCourt(ctx context.Context, id uuid.UUID) (Court, error)
	// CountWaitingPlayers is a read-only headcount (no locking); it is only
	// used to decide how many courts the auto-fill job should attempt this
	// tick, not to guarantee correctness — that still comes from
	// match.Service.GenerateAutomatic's own transactional locking.
	CountWaitingPlayers(ctx context.Context, sessionID uuid.UUID) (int64, error)
}

type pgRepository struct {
	q *db.Queries
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &pgRepository{q: db.New(pool)}
}

func (r *pgRepository) CreateSession(ctx context.Context, clubID uuid.UUID, name string, mode AssignmentMode) (Session, error) {
	row, err := r.q.CreateSession(ctx, db.CreateSessionParams{
		ClubID:         clubID,
		Name:           name,
		AssignmentMode: db.SessionAssignmentMode(mode),
	})
	if err != nil {
		return Session{}, fmt.Errorf("create session: %w", err)
	}
	return toSession(row), nil
}

func (r *pgRepository) GetByID(ctx context.Context, id uuid.UUID) (Session, error) {
	row, err := r.q.GetSessionByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Session{}, ErrNotFound
		}
		return Session{}, fmt.Errorf("get session: %w", err)
	}
	return toSession(row), nil
}

func (r *pgRepository) StartSession(ctx context.Context, id uuid.UUID) (Session, error) {
	row, err := r.q.StartSession(ctx, id)
	if err != nil {
		return Session{}, fmt.Errorf("start session: %w", err)
	}
	return toSession(row), nil
}

func (r *pgRepository) EndSession(ctx context.Context, id uuid.UUID) (Session, error) {
	row, err := r.q.EndSession(ctx, id)
	if err != nil {
		return Session{}, fmt.Errorf("end session: %w", err)
	}
	return toSession(row), nil
}

func (r *pgRepository) UpdateAssignmentMode(ctx context.Context, id uuid.UUID, mode AssignmentMode) (Session, error) {
	row, err := r.q.UpdateSessionAssignmentMode(ctx, db.UpdateSessionAssignmentModeParams{
		ID:             id,
		AssignmentMode: db.SessionAssignmentMode(mode),
	})
	if err != nil {
		return Session{}, fmt.Errorf("update session assignment mode: %w", err)
	}
	return toSession(row), nil
}

func (r *pgRepository) UpdateAutoFillEnabled(ctx context.Context, id uuid.UUID, enabled bool) (Session, error) {
	row, err := r.q.UpdateSessionAutoFillEnabled(ctx, db.UpdateSessionAutoFillEnabledParams{
		ID:              id,
		AutoFillEnabled: enabled,
	})
	if err != nil {
		return Session{}, fmt.Errorf("update session auto fill enabled: %w", err)
	}
	return toSession(row), nil
}

func (r *pgRepository) ListByClub(ctx context.Context, clubID uuid.UUID) ([]Session, error) {
	rows, err := r.q.ListSessionsByClub(ctx, clubID)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	sessions := make([]Session, 0, len(rows))
	for _, row := range rows {
		sessions = append(sessions, toSession(row))
	}
	return sessions, nil
}

func (r *pgRepository) ListActiveAutoFillSessions(ctx context.Context) ([]Session, error) {
	rows, err := r.q.ListActiveAutoFillSessions(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active auto-fill sessions: %w", err)
	}
	sessions := make([]Session, 0, len(rows))
	for _, row := range rows {
		sessions = append(sessions, toSession(row))
	}
	return sessions, nil
}

func (r *pgRepository) AddSessionPlayer(ctx context.Context, sessionID, playerID uuid.UUID) (Player, error) {
	row, err := r.q.AddSessionPlayer(ctx, db.AddSessionPlayerParams{SessionID: sessionID, PlayerID: playerID})
	if err != nil {
		return Player{}, fmt.Errorf("add session player: %w", err)
	}
	return toPlayer(row), nil
}

func (r *pgRepository) GetSessionPlayer(ctx context.Context, id uuid.UUID) (Player, error) {
	row, err := r.q.GetSessionPlayer(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Player{}, ErrNotFound
		}
		return Player{}, fmt.Errorf("get session player: %w", err)
	}
	return toPlayer(row), nil
}

func (r *pgRepository) GetSessionPlayerBySessionAndPlayer(ctx context.Context, sessionID, playerID uuid.UUID) (Player, error) {
	row, err := r.q.GetSessionPlayerBySessionAndPlayer(ctx, db.GetSessionPlayerBySessionAndPlayerParams{
		SessionID: sessionID,
		PlayerID:  playerID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Player{}, ErrNotFound
		}
		return Player{}, fmt.Errorf("get session player: %w", err)
	}
	return toPlayer(row), nil
}

func (r *pgRepository) ListSessionPlayers(ctx context.Context, sessionID uuid.UUID) ([]Player, error) {
	rows, err := r.q.ListSessionPlayers(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list session players: %w", err)
	}
	players := make([]Player, 0, len(rows))
	for _, row := range rows {
		players = append(players, toPlayer(row))
	}
	return players, nil
}

func (r *pgRepository) SetSessionPlayerStatus(ctx context.Context, id uuid.UUID, status PlayerStatus) (Player, error) {
	row, err := r.q.SetSessionPlayerStatus(ctx, db.SetSessionPlayerStatusParams{
		ID:     id,
		Status: db.SessionPlayerStatus(status),
	})
	if err != nil {
		return Player{}, fmt.Errorf("set session player status: %w", err)
	}
	return toPlayer(row), nil
}

func (r *pgRepository) CreateCourt(ctx context.Context, sessionID uuid.UUID, name string) (Court, error) {
	row, err := r.q.CreateCourt(ctx, db.CreateCourtParams{SessionID: sessionID, Name: name})
	if err != nil {
		return Court{}, fmt.Errorf("create court: %w", err)
	}
	return toCourt(row), nil
}

func (r *pgRepository) ListCourts(ctx context.Context, sessionID uuid.UUID) ([]Court, error) {
	rows, err := r.q.ListCourtsBySession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list courts: %w", err)
	}
	courts := make([]Court, 0, len(rows))
	for _, row := range rows {
		courts = append(courts, toCourt(row))
	}
	return courts, nil
}

func (r *pgRepository) ListAvailableCourts(ctx context.Context, sessionID uuid.UUID) ([]Court, error) {
	rows, err := r.q.ListAvailableCourtsBySession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list available courts: %w", err)
	}
	courts := make([]Court, 0, len(rows))
	for _, row := range rows {
		courts = append(courts, toCourt(row))
	}
	return courts, nil
}

func (r *pgRepository) GetCourt(ctx context.Context, id uuid.UUID) (Court, error) {
	row, err := r.q.GetCourtByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Court{}, ErrNotFound
		}
		return Court{}, fmt.Errorf("get court: %w", err)
	}
	return toCourt(row), nil
}

func (r *pgRepository) CountWaitingPlayers(ctx context.Context, sessionID uuid.UUID) (int64, error) {
	count, err := r.q.CountWaitingSessionPlayers(ctx, sessionID)
	if err != nil {
		return 0, fmt.Errorf("count waiting session players: %w", err)
	}
	return count, nil
}

func toSession(row db.Session) Session {
	s := Session{
		ID:              row.ID,
		ClubID:          row.ClubID,
		Name:            row.Name,
		Status:          Status(row.Status),
		AssignmentMode:  AssignmentMode(row.AssignmentMode),
		AutoFillEnabled: row.AutoFillEnabled,
		CreatedAt:       row.CreatedAt.Time,
	}
	if row.StartedAt.Valid {
		t := row.StartedAt.Time
		s.StartedAt = &t
	}
	if row.EndedAt.Valid {
		t := row.EndedAt.Time
		s.EndedAt = &t
	}
	return s
}

func toPlayer(row db.SessionPlayer) Player {
	return Player{
		ID:                        row.ID,
		SessionID:                 row.SessionID,
		PlayerID:                  row.PlayerID,
		Status:                    PlayerStatus(row.Status),
		WaitingStartedAt:          row.WaitingStartedAt.Time,
		AccumulatedWaitingSeconds: row.AccumulatedWaitingSeconds,
		MatchesPlayed:             row.MatchesPlayed,
		Wins:                      row.Wins,
		Losses:                    row.Losses,
	}
}

func toCourt(row db.Court) Court {
	return Court{
		ID:        row.ID,
		SessionID: row.SessionID,
		Name:      row.Name,
		Status:    CourtStatus(row.Status),
	}
}
