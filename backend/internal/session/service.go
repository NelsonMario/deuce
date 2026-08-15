package session

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	"deuce/backend/internal/apperr"
	"deuce/backend/internal/auth"
	"deuce/backend/internal/club"
	"deuce/backend/internal/device"
	"deuce/backend/internal/player"
	"deuce/backend/pkg/idgen"
)

type Service struct {
	repo    Repository
	clubs   club.Repository
	players player.Repository
	hasher  auth.Hasher
	devices device.Linker
	logger  *slog.Logger
}

func NewService(repo Repository, clubs club.Repository, players player.Repository, hasher auth.Hasher, devices device.Linker, logger *slog.Logger) *Service {
	return &Service{repo: repo, clubs: clubs, players: players, hasher: hasher, devices: devices, logger: logger}
}

type CreateSessionInput struct {
	ClubID         uuid.UUID
	HostPlayerID   uuid.UUID
	Name           string
	AssignmentMode AssignmentMode
}

func (s *Service) CreateSession(ctx context.Context, in CreateSessionInput) (Session, error) {
	mode := in.AssignmentMode
	if mode == "" {
		mode = AssignmentAutomatic
	}
	if !mode.Valid() {
		return Session{}, apperr.Validation("invalid assignment_mode")
	}
	sess, err := s.repo.CreateSession(ctx, in.ClubID, in.Name, mode)
	if err != nil {
		return Session{}, apperr.Internal(err)
	}
	if in.HostPlayerID != uuid.Nil {
		if _, err := s.repo.AddSessionPlayer(ctx, sess.ID, in.HostPlayerID); err != nil {
			return Session{}, apperr.Internal(err)
		}
	}
	return sess, nil
}

func (s *Service) GetSession(ctx context.Context, id uuid.UUID) (Session, error) {
	sess, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Session{}, apperr.NotFound("session")
		}
		return Session{}, apperr.Internal(err)
	}
	return sess, nil
}

// ListByClub returns every session belonging to a club, newest first — the
// roster shown on the club page, so any co-host device sees sessions
// created from any other device, not just its own local history.
func (s *Service) ListByClub(ctx context.Context, clubID uuid.UUID) ([]Session, error) {
	sessions, err := s.repo.ListByClub(ctx, clubID)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return sessions, nil
}

func (s *Service) StartSession(ctx context.Context, id uuid.UUID) (Session, error) {
	sess, err := s.GetSession(ctx, id)
	if err != nil {
		return Session{}, err
	}
	if sess.Status != StatusNotStarted {
		return Session{}, apperr.InvalidState("session is not in NOT_STARTED state")
	}
	updated, err := s.repo.StartSession(ctx, id)
	if err != nil {
		return Session{}, apperr.Internal(err)
	}
	s.logger.Info("session_started", "session_id", id)
	return updated, nil
}

func (s *Service) EndSession(ctx context.Context, id uuid.UUID) (Session, error) {
	sess, err := s.GetSession(ctx, id)
	if err != nil {
		return Session{}, err
	}
	if sess.Status != StatusActive {
		return Session{}, apperr.InvalidState("session is not ACTIVE")
	}
	updated, err := s.repo.EndSession(ctx, id)
	if err != nil {
		return Session{}, apperr.Internal(err)
	}
	s.logger.Info("session_ended", "session_id", id)
	return updated, nil
}

// SetAssignmentMode lets a host toggle a session's matchmaking assignment
// mode (AUTOMATIC/MANUAL) at any point in the session lifecycle, including
// while ACTIVE — unlike Start/EndSession, no status check is required here
// since the mode only affects how the *next* match is generated. Host
// authorization is enforced by the caller (see
// handler.requireHostOfSession), mirroring StartSession/EndSession.
func (s *Service) SetAssignmentMode(ctx context.Context, id uuid.UUID, mode AssignmentMode) (Session, error) {
	if !mode.Valid() {
		return Session{}, apperr.Validation("invalid assignment_mode")
	}
	if _, err := s.GetSession(ctx, id); err != nil {
		return Session{}, err
	}
	updated, err := s.repo.UpdateAssignmentMode(ctx, id, mode)
	if err != nil {
		return Session{}, apperr.Internal(err)
	}
	s.logger.Info("session_assignment_mode_changed", "session_id", id, "mode", mode)
	return updated, nil
}

// SetAutoFillEnabled lets a host toggle whether the fully-automatic auto-fill
// background job (see internal/match/autofill.go) should keep filling this
// session's empty courts with no per-court "Generate match" trigger. Like
// SetAssignmentMode, this may be called at any point in the session
// lifecycle, including while ACTIVE — it only affects whether *future* ticks
// of the background job touch this session, so no status check is required.
// Host authorization is enforced by the caller (see
// handler.requireHostOfSession).
func (s *Service) SetAutoFillEnabled(ctx context.Context, id uuid.UUID, enabled bool) (Session, error) {
	if _, err := s.GetSession(ctx, id); err != nil {
		return Session{}, err
	}
	updated, err := s.repo.UpdateAutoFillEnabled(ctx, id, enabled)
	if err != nil {
		return Session{}, apperr.Internal(err)
	}
	s.logger.Info("session_auto_fill_enabled_changed", "session_id", id, "auto_fill_enabled", enabled)
	return updated, nil
}

func (s *Service) ListCourts(ctx context.Context, sessionID uuid.UUID) ([]Court, error) {
	courts, err := s.repo.ListCourts(ctx, sessionID)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return courts, nil
}

func (s *Service) CreateCourt(ctx context.Context, sessionID uuid.UUID, name string) (Court, error) {
	if name == "" {
		return Court{}, apperr.Validation("court name is required")
	}
	c, err := s.repo.CreateCourt(ctx, sessionID, name)
	if err != nil {
		return Court{}, apperr.Internal(err)
	}
	return c, nil
}

func (s *Service) ListPlayers(ctx context.Context, sessionID uuid.UUID) ([]Player, error) {
	players, err := s.repo.ListSessionPlayers(ctx, sessionID)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return players, nil
}

func (s *Service) GetSessionPlayer(ctx context.Context, id uuid.UUID) (Player, error) {
	sp, err := s.repo.GetSessionPlayer(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Player{}, apperr.NotFound("session player")
		}
		return Player{}, apperr.Internal(err)
	}
	return sp, nil
}

type JoinSessionInput struct {
	JoinCode    string
	DisplayName string
	Gender      player.Gender
	// DeviceID is optional. When it matches a previous join/create for this
	// session's club, the caller is recognized as that same player (rating/
	// history carry over) instead of getting a brand-new identity.
	DeviceID string
}

type JoinSessionResult struct {
	Session       Session
	SessionPlayer Player
	Player        player.Player
	Token         string
	IsNewJoin     bool
}

// JoinSession is the primary accountless entry point described in spec
// section 5 (scan QR -> enter name -> select gender -> join session). It
// validates the club join code, either recognizes a returning device (see
// internal/device) or creates a brand-new player identity, adds club
// membership, puts the player into WAITING for this session, and issues a
// fresh session token either way.
func (s *Service) JoinSession(ctx context.Context, sessionID uuid.UUID, in JoinSessionInput) (JoinSessionResult, error) {
	if in.DisplayName == "" {
		return JoinSessionResult{}, apperr.Validation("display name is required")
	}
	if !in.Gender.Valid() {
		return JoinSessionResult{}, apperr.Validation("invalid gender")
	}

	sess, err := s.GetSession(ctx, sessionID)
	if err != nil {
		return JoinSessionResult{}, err
	}
	if sess.Status == StatusFinished {
		return JoinSessionResult{}, apperr.InvalidState("session has finished")
	}

	c, err := s.clubs.GetByID(ctx, sess.ClubID)
	if err != nil {
		return JoinSessionResult{}, apperr.Internal(err)
	}
	if c.JoinCode != in.JoinCode {
		return JoinSessionResult{}, apperr.Unauthorized("invalid join code")
	}

	p, isNewJoin, err := s.resolvePlayer(ctx, in.DeviceID, c.ID, in.DisplayName, in.Gender)
	if err != nil {
		return JoinSessionResult{}, err
	}
	if _, err := s.clubs.AddMember(ctx, c.ID, p.ID, club.RolePlayer); err != nil {
		return JoinSessionResult{}, apperr.Internal(err)
	}

	sp, err := s.repo.AddSessionPlayer(ctx, sessionID, p.ID)
	if err != nil {
		return JoinSessionResult{}, apperr.Internal(err)
	}

	raw, err := idgen.PlayerToken()
	if err != nil {
		return JoinSessionResult{}, apperr.Internal(err)
	}
	if err := s.players.CreateToken(ctx, p.ID, s.hasher.Hash(raw)); err != nil {
		return JoinSessionResult{}, apperr.Internal(err)
	}

	s.logger.Info("player_joined", "session_id", sessionID, "player_id", p.ID, "returning", !isNewJoin)

	return JoinSessionResult{Session: sess, SessionPlayer: sp, Player: p, Token: raw, IsNewJoin: isNewJoin}, nil
}

// resolvePlayer wraps device.Resolve.
func (s *Service) resolvePlayer(ctx context.Context, deviceID string, clubID uuid.UUID, displayName string, gender player.Gender) (player.Player, bool, error) {
	p, isNewJoin, err := device.Resolve(ctx, s.devices, s.players, deviceID, clubID, displayName, gender)
	if err != nil {
		return player.Player{}, false, apperr.Internal(err)
	}
	return p, isNewJoin, nil
}

type GuestInput struct {
	DisplayName string
	Gender      player.Gender
}

type RegisterGuestsInput struct {
	SessionID uuid.UUID
	Guests    []GuestInput
}

// RegisterGuests adds players the host registered on behalf of people who
// didn't join via the invite link. Guests are ordinary player identities
// flagged is_guest with a normal starting rating of 1000. Re-registering a
// name already present in the club reuses that player (case-insensitive) and
// refreshes their profile, so a returning guest keeps one row instead of
// accumulating duplicates.
func (s *Service) RegisterGuests(ctx context.Context, in RegisterGuestsInput) ([]Player, error) {
	if len(in.Guests) == 0 {
		return nil, apperr.Validation("at least one guest is required")
	}
	sess, err := s.GetSession(ctx, in.SessionID)
	if err != nil {
		return nil, err
	}
	if sess.Status == StatusFinished {
		return nil, apperr.InvalidState("session has finished")
	}

	type guest struct {
		name   string
		gender player.Gender
	}
	clean := make([]guest, 0, len(in.Guests))
	for _, g := range in.Guests {
		name := strings.TrimSpace(g.DisplayName)
		if name == "" {
			return nil, apperr.Validation("guest name cannot be empty")
		}
		if !g.Gender.Valid() {
			return nil, apperr.Validation("invalid gender")
		}
		clean = append(clean, guest{name: name, gender: g.Gender})
	}

	results := make([]Player, 0, len(clean))
	for _, g := range clean {
		p, err := s.resolveGuestPlayer(ctx, sess.ClubID, g.name, g.gender)
		if err != nil {
			return nil, err
		}
		sp, err := s.repo.AddSessionPlayer(ctx, in.SessionID, p.ID)
		if err != nil {
			return nil, apperr.Internal(err)
		}
		results = append(results, sp)
	}
	s.logger.Info("guests_registered", "session_id", in.SessionID, "count", len(results))
	return results, nil
}

// resolveGuestPlayer returns an existing club member with the same display
// name (case-insensitive) or creates a new guest identity. Reusing a member
// refreshes their profile (and updated_at) so a regularly-returning guest
// never ages into the cron cleanup window.
func (s *Service) resolveGuestPlayer(ctx context.Context, clubID uuid.UUID, name string, gender player.Gender) (player.Player, error) {
	if existing, err := s.clubs.FindMemberPlayerByName(ctx, clubID, name); err == nil {
		updated, err := s.players.UpdateProfile(ctx, existing.ID, name, gender)
		if err != nil {
			return player.Player{}, apperr.Internal(err)
		}
		return updated, nil
	} else if !errors.Is(err, club.ErrNotFound) {
		return player.Player{}, apperr.Internal(err)
	}

	p, err := s.players.CreateGuest(ctx, name, gender)
	if err != nil {
		return player.Player{}, apperr.Internal(err)
	}
	if _, err := s.players.CreateRating(ctx, p.ID, player.InitialRating); err != nil {
		return player.Player{}, apperr.Internal(err)
	}
	if _, err := s.clubs.AddMember(ctx, clubID, p.ID, club.RolePlayer); err != nil {
		return player.Player{}, apperr.Internal(err)
	}
	return p, nil
}

// SetPlayerStatus handles player/host-initiated transitions:
// WAITING<->BREAK, and WAITING/BREAK->ENDED. PLAYING is exclusively
// backend-controlled via match generation/finish and cannot be set here.
func (s *Service) SetPlayerStatus(ctx context.Context, sessionPlayerID uuid.UUID, to PlayerStatus) (Player, error) {
	if !to.Valid() {
		return Player{}, apperr.Validation("invalid status")
	}
	if to == PlayerPlaying {
		return Player{}, apperr.Validation("PLAYING is backend-controlled and cannot be set directly")
	}

	sp, err := s.GetSessionPlayer(ctx, sessionPlayerID)
	if err != nil {
		return Player{}, err
	}
	if !sp.Status.CanTransitionTo(to) {
		return Player{}, apperr.InvalidState("invalid session player state transition: " + string(sp.Status) + " -> " + string(to))
	}

	updated, err := s.repo.SetSessionPlayerStatus(ctx, sessionPlayerID, to)
	if err != nil {
		return Player{}, apperr.Internal(err)
	}
	return updated, nil
}
