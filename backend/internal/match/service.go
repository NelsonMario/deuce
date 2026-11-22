package match

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"deuce/backend/internal/apperr"
	"deuce/backend/internal/database"
	"deuce/backend/internal/database/db"
	"deuce/backend/internal/lock"
	"deuce/backend/internal/matchmaking"
	"deuce/backend/internal/rating"
)

// Service owns the concurrency-critical match lifecycle. Match generation
// and match finishing each run inside a single PostgreSQL transaction with
// explicit row locking (SELECT ... FOR UPDATE) so that:
//   - the same session_player can never be assigned to two matches at once
//     (guaranteed by locking every WAITING session_player row for the
//     session before selecting any of them), and
//   - a match can never finish without its rating updates, or vice versa
//     (both happen in the same transaction).
//
// Announcement contains the public information that belongs in a group
// message after a match is assigned. It deliberately has no WhatsApp types:
// match remains independent of the delivery adapter.
type Announcement struct {
	SessionID uuid.UUID
	MatchID   uuid.UUID
	CourtName string
	Format    string
	TeamA     [2]string
	TeamB     [2]string
}

// AnnouncementOutbox persists an announcement in the caller's transaction.
// A failed or disconnected WhatsApp client must never affect matchmaking.
type AnnouncementOutbox interface {
	Enqueue(ctx context.Context, tx pgx.Tx, announcement Announcement) error
}

type Service struct {
	pool   *pgxpool.Pool
	reads  ReadRepository
	locker lock.Locker
	logger *slog.Logger
	outbox AnnouncementOutbox
}

func NewService(pool *pgxpool.Pool, reads ReadRepository, locker lock.Locker, logger *slog.Logger) *Service {
	return &Service{pool: pool, reads: reads, locker: locker, logger: logger}
}

func (s *Service) SetAnnouncementOutbox(outbox AnnouncementOutbox) {
	s.outbox = outbox
}

// matchGenLockTTL bounds how long a double-tap guard entry can survive if
// its Release (deferred in GenerateAutomatic/ConfirmManual) is never
// reached — e.g. the process crashes mid-transaction. A few seconds is
// plenty: it only needs to outlast the window between a duplicate request
// arriving and the first request's Postgres transaction acquiring its own
// row locks.
const matchGenLockTTL = 5 * time.Second

// matchGenLockKey scopes the double-tap guard to one court within one
// session, matching the granularity of the Postgres court lock it sits in
// front of.
func matchGenLockKey(sessionID, courtID uuid.UUID) string {
	return fmt.Sprintf("matchgen:session:%s:court:%s", sessionID, courtID)
}

// sessionPlayerID extracts the player id from a session_players row fetched
// as a live matchmaking candidate (ListWaitingSessionPlayersForUpdate,
// LockSessionPlayersByIDs). player_id is nullable at the schema level for
// historical compatibility, but both call sites select current players in
// an ACTIVE session, so it is always non-null here.
func sessionPlayerID(sp db.SessionPlayer) uuid.UUID {
	return uuid.UUID(sp.PlayerID.Bytes)
}

// matchPlayerID is sessionPlayerID's counterpart for match_players rows
// fetched while a match is still PLAYING (FinishMatch): those were added by
// AddMatchPlayer moments earlier for players in what is necessarily still
// an ACTIVE session, so player_id is always non-null here too.
func matchPlayerID(p db.MatchPlayer) uuid.UUID {
	return uuid.UUID(p.PlayerID.Bytes)
}

func (s *Service) GetMatch(ctx context.Context, id uuid.UUID) (Match, error) {
	m, err := s.reads.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Match{}, apperr.NotFound("match")
		}
		return Match{}, apperr.Internal(err)
	}
	return m, nil
}

func (s *Service) ListPlayers(ctx context.Context, matchID uuid.UUID) ([]Player, error) {
	players, err := s.reads.ListPlayers(ctx, matchID)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return players, nil
}

func (s *Service) ListBySession(ctx context.Context, sessionID uuid.UUID) ([]Match, error) {
	matches, err := s.reads.ListBySession(ctx, sessionID)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return matches, nil
}

// ============================================================
// Automatic generation
// ============================================================

type GenerateInput struct {
	SessionID uuid.UUID
	CourtID   uuid.UUID
	Format    Format
}

type PreviewAutoInput struct {
	SessionID uuid.UUID
	Format    Format
}

func (s *Service) PreviewAutomatic(ctx context.Context, in PreviewAutoInput) (*Proposal, error) {
	if !in.Format.Valid() {
		return nil, apperr.Validation("invalid match format")
	}

	q := db.New(s.pool)
	waiting, err := q.ListWaitingSessionPlayersForUpdate(ctx, in.SessionID)
	if err != nil {
		return nil, fmt.Errorf("list waiting session players: %w", err)
	}

	now := time.Now()
	candidates := make([]matchmaking.Candidate, 0, len(waiting))
	for _, sp := range waiting {
		playerID := sessionPlayerID(sp)
		p, err := q.GetPlayer(ctx, playerID)
		if err != nil {
			return nil, fmt.Errorf("get player %s: %w", playerID, err)
		}
		r, err := q.GetPlayerRating(ctx, playerID)
		if err != nil {
			return nil, fmt.Errorf("get player rating %s: %w", playerID, err)
		}
		waitSeconds := float64(sp.AccumulatedWaitingSeconds) + now.Sub(sp.WaitingStartedAt.Time).Seconds()
		candidates = append(candidates, matchmaking.Candidate{
			PlayerID:       playerID.String(),
			Rating:         r.Rating,
			Gender:         matchmaking.Gender(p.Gender),
			Status:         matchmaking.StatusWaiting,
			MatchesPlayed:  int(sp.MatchesPlayed),
			WaitingSeconds: waitSeconds,
		})
	}

	proposal, err := matchmaking.GenerateMatch(candidates, matchmaking.Format(in.Format))
	if err != nil {
		if errors.Is(err, matchmaking.ErrInsufficientPlayers) {
			return nil, apperr.InsufficientPlayers("not enough eligible WAITING players for " + string(in.Format))
		}
		return nil, err
	}
	return toProposal(in.Format, proposal), nil
}

// GenerateAutomatic implements spec section 13: lock every WAITING
// session_player for the session, run the deterministic matchmaking
// algorithm over them, lock the target court, and atomically create the
// match + flip the chosen four players to PLAYING. The match is created but
// NOT started — the host must call StartMatch explicitly.
func (s *Service) GenerateAutomatic(ctx context.Context, in GenerateInput) (Match, error) {
	lockKey := matchGenLockKey(in.SessionID, in.CourtID)
	if !s.locker.TryAcquire(ctx, lockKey) {
		return Match{}, apperr.GenerationInProgress("a match generation request for this court is already in progress")
	}
	defer s.locker.Release(ctx, lockKey)

	if !in.Format.Valid() {
		return Match{}, apperr.Validation("invalid match format")
	}

	var result Match
	now := time.Now()

	err := database.RunInTxWithTx(ctx, s.pool, func(tx pgx.Tx, q *db.Queries) error {
		waiting, err := q.ListWaitingSessionPlayersForUpdate(ctx, in.SessionID)
		if err != nil {
			return fmt.Errorf("lock waiting session players: %w", err)
		}

		candidates := make([]matchmaking.Candidate, 0, len(waiting))
		bySessionPlayer := map[uuid.UUID]db.SessionPlayer{}
		for _, sp := range waiting {
			playerID := sessionPlayerID(sp)
			p, err := q.GetPlayer(ctx, playerID)
			if err != nil {
				return fmt.Errorf("get player %s: %w", playerID, err)
			}
			r, err := q.GetPlayerRating(ctx, playerID)
			if err != nil {
				return fmt.Errorf("get player rating %s: %w", playerID, err)
			}
			waitSeconds := float64(sp.AccumulatedWaitingSeconds) + now.Sub(sp.WaitingStartedAt.Time).Seconds()
			candidates = append(candidates, matchmaking.Candidate{
				PlayerID:       playerID.String(),
				Rating:         r.Rating,
				Gender:         matchmaking.Gender(p.Gender),
				Status:         matchmaking.StatusWaiting,
				MatchesPlayed:  int(sp.MatchesPlayed),
				WaitingSeconds: waitSeconds,
			})
			bySessionPlayer[playerID] = sp
		}

		proposal, err := matchmaking.GenerateMatch(candidates, matchmaking.Format(in.Format))
		if err != nil {
			if errors.Is(err, matchmaking.ErrInsufficientPlayers) {
				return apperr.InsufficientPlayers("not enough eligible WAITING players for " + string(in.Format))
			}
			return err
		}

		court, err := q.LockAvailableCourtForUpdate(ctx, in.CourtID)
		if err != nil {
			return apperr.Conflict("court is not available")
		}
		if court.SessionID != in.SessionID {
			return apperr.Validation("court does not belong to this session")
		}

		m, err := q.CreateMatch(ctx, db.CreateMatchParams{
			SessionID: in.SessionID,
			CourtID:   court.ID,
			Format:    db.MatchFormat(in.Format),
		})
		if err != nil {
			return fmt.Errorf("create match: %w", err)
		}

		if err := assignTeams(ctx, q, m.ID, proposal, bySessionPlayer); err != nil {
			return err
		}

		if _, err := q.SetCourtStatus(ctx, db.SetCourtStatusParams{ID: court.ID, Status: db.CourtStatusPLAYING}); err != nil {
			return fmt.Errorf("set court playing: %w", err)
		}

		result = toMatch(m)
		result.Players = []uuid.UUID{
			uuid.MustParse(proposal.TeamA[0].PlayerID),
			uuid.MustParse(proposal.TeamA[1].PlayerID),
			uuid.MustParse(proposal.TeamB[0].PlayerID),
			uuid.MustParse(proposal.TeamB[1].PlayerID),
		}
		if s.outbox != nil {
			ids := [4]uuid.UUID{uuid.MustParse(proposal.TeamA[0].PlayerID), uuid.MustParse(proposal.TeamA[1].PlayerID), uuid.MustParse(proposal.TeamB[0].PlayerID), uuid.MustParse(proposal.TeamB[1].PlayerID)}
			names := make(map[uuid.UUID]string, 4)
			for _, id := range ids {
				p, err := q.GetPlayer(ctx, id)
				if err != nil {
					return fmt.Errorf("get player name: %w", err)
				}
				names[id] = p.DisplayName
			}
			if err := s.outbox.Enqueue(ctx, tx, Announcement{SessionID: in.SessionID, MatchID: m.ID, CourtName: court.Name, Format: string(in.Format), TeamA: [2]string{names[ids[0]], names[ids[1]]}, TeamB: [2]string{names[ids[2]], names[ids[3]]}}); err != nil {
				return fmt.Errorf("enqueue group announcement: %w", err)
			}
		}
		return nil
	})

	if err != nil {
		if appErr, ok := apperr.As(err); ok {
			return Match{}, appErr
		}
		return Match{}, apperr.Internal(err)
	}

	s.logger.Info("match_generated", "session_id", in.SessionID, "match_id", result.ID, "format", in.Format)
	return result, nil
}

// assignTeams creates match_players for the proposal's four players and
// flips their session_player rows from WAITING to PLAYING. Called from
// within an active transaction that already holds the necessary locks.
func assignTeams(ctx context.Context, q *db.Queries, matchID uuid.UUID, proposal *matchmaking.Proposal, bySessionPlayer map[uuid.UUID]db.SessionPlayer) error {
	assignments := []struct {
		team matchmaking.TeamAssignment
		side db.MatchTeam
	}{
		{proposal.TeamA[0], db.MatchTeamA},
		{proposal.TeamA[1], db.MatchTeamA},
		{proposal.TeamB[0], db.MatchTeamB},
		{proposal.TeamB[1], db.MatchTeamB},
	}

	for _, a := range assignments {
		playerID, err := uuid.Parse(a.team.PlayerID)
		if err != nil {
			return fmt.Errorf("parse player id: %w", err)
		}
		if _, err := q.AddMatchPlayer(ctx, db.AddMatchPlayerParams{
			MatchID:      matchID,
			PlayerID:     playerID,
			Team:         a.side,
			RatingBefore: pgtype.Float8{Float64: a.team.Rating, Valid: true},
		}); err != nil {
			return fmt.Errorf("add match player: %w", err)
		}

		sp, ok := bySessionPlayer[playerID]
		if !ok {
			return fmt.Errorf("session player not found for %s", playerID)
		}
		if sp.Status != db.SessionPlayerStatusWAITING {
			// Guarded by the FOR UPDATE lock taken before selection, so this
			// should be unreachable; kept as a defensive invariant check.
			return apperr.Conflict("player is no longer WAITING")
		}
		if _, err := q.SetSessionPlayerStatus(ctx, db.SetSessionPlayerStatusParams{
			ID:     sp.ID,
			Status: db.SessionPlayerStatusPLAYING,
		}); err != nil {
			return fmt.Errorf("set session player playing: %w", err)
		}
	}
	return nil
}

// ============================================================
// Manual assignment
// ============================================================

type RecommendInput struct {
	SessionID uuid.UUID
	PlayerIDs [4]uuid.UUID
	Format    Format
}

// RecommendManual is read-only: it fetches the four host-selected players
// and returns the balanced team-split recommendation. It does not lock or
// reserve anyone; ConfirmManual performs the actual atomic assignment.
func (s *Service) RecommendManual(ctx context.Context, in RecommendInput) (*Proposal, error) {
	if !in.Format.Valid() {
		return nil, apperr.Validation("invalid match format")
	}

	q := db.New(s.pool)
	var candidates [4]matchmaking.Candidate
	now := time.Now()

	for i, playerID := range in.PlayerIDs {
		sp, err := q.GetSessionPlayerBySessionAndPlayer(ctx, db.GetSessionPlayerBySessionAndPlayerParams{
			SessionID: in.SessionID,
			PlayerID:  playerID,
		})
		if err != nil {
			return nil, apperr.PlayerNotEligible("player is not part of this session")
		}
		if sp.Status != db.SessionPlayerStatusWAITING {
			return nil, apperr.PlayerNotEligible("player is not in WAITING state")
		}
		p, err := q.GetPlayer(ctx, playerID)
		if err != nil {
			return nil, apperr.Internal(err)
		}
		r, err := q.GetPlayerRating(ctx, playerID)
		if err != nil {
			return nil, apperr.Internal(err)
		}
		waitSeconds := float64(sp.AccumulatedWaitingSeconds) + now.Sub(sp.WaitingStartedAt.Time).Seconds()
		candidates[i] = matchmaking.Candidate{
			PlayerID:       playerID.String(),
			Rating:         r.Rating,
			Gender:         matchmaking.Gender(p.Gender),
			Status:         matchmaking.StatusWaiting,
			MatchesPlayed:  int(sp.MatchesPlayed),
			WaitingSeconds: waitSeconds,
		}
	}

	proposal, err := matchmaking.RecommendSplit(candidates, matchmaking.Format(in.Format))
	if err != nil {
		return nil, apperr.Validation(err.Error())
	}
	return toProposal(in.Format, proposal), nil
}

type ConfirmManualInput struct {
	SessionID uuid.UUID
	CourtID   uuid.UUID
	Format    Format
	TeamA     [2]uuid.UUID
	TeamB     [2]uuid.UUID
}

// ConfirmManual atomically reserves the host's four chosen players (which
// may be an override of the recommendation) and creates the match, using
// the same locking discipline as GenerateAutomatic.
func (s *Service) ConfirmManual(ctx context.Context, in ConfirmManualInput) (Match, error) {
	lockKey := matchGenLockKey(in.SessionID, in.CourtID)
	if !s.locker.TryAcquire(ctx, lockKey) {
		return Match{}, apperr.GenerationInProgress("a match generation request for this court is already in progress")
	}
	defer s.locker.Release(ctx, lockKey)

	if !in.Format.Valid() {
		return Match{}, apperr.Validation("invalid match format")
	}

	ids := []uuid.UUID{in.TeamA[0], in.TeamA[1], in.TeamB[0], in.TeamB[1]}
	if err := requireDistinct(ids); err != nil {
		return Match{}, err
	}

	var result Match
	err := database.RunInTxWithTx(ctx, s.pool, func(tx pgx.Tx, q *db.Queries) error {
		locked, err := q.LockSessionPlayersByIDs(ctx, mustSessionPlayerIDs(ctx, q, in.SessionID, ids))
		if err != nil {
			return fmt.Errorf("lock session players: %w", err)
		}
		if len(locked) != 4 {
			return apperr.PlayerNotEligible("one or more selected players are not part of this session")
		}

		byPlayer := map[uuid.UUID]db.SessionPlayer{}
		ratings := map[uuid.UUID]float64{}
		for _, sp := range locked {
			if sp.SessionID != in.SessionID {
				return apperr.PlayerNotEligible("player does not belong to this session")
			}
			if sp.Status != db.SessionPlayerStatusWAITING {
				return apperr.PlayerNotEligible("player is not in WAITING state")
			}
			playerID := sessionPlayerID(sp)
			byPlayer[playerID] = sp
			r, err := q.GetPlayerRating(ctx, playerID)
			if err != nil {
				return fmt.Errorf("get rating: %w", err)
			}
			ratings[playerID] = r.Rating
		}

		genders := map[uuid.UUID]matchmaking.Gender{}
		for _, id := range ids {
			p, err := q.GetPlayer(ctx, id)
			if err != nil {
				return fmt.Errorf("get player: %w", err)
			}
			genders[id] = matchmaking.Gender(p.Gender)
		}

		if err := validateFormatGenders(in.Format, [2]matchmaking.Gender{genders[in.TeamA[0]], genders[in.TeamA[1]]}, [2]matchmaking.Gender{genders[in.TeamB[0]], genders[in.TeamB[1]]}); err != nil {
			return err
		}

		court, err := q.LockAvailableCourtForUpdate(ctx, in.CourtID)
		if err != nil {
			return apperr.Conflict("court is not available")
		}
		if court.SessionID != in.SessionID {
			return apperr.Validation("court does not belong to this session")
		}

		m, err := q.CreateMatch(ctx, db.CreateMatchParams{
			SessionID: in.SessionID,
			CourtID:   court.ID,
			Format:    db.MatchFormat(in.Format),
		})
		if err != nil {
			return fmt.Errorf("create match: %w", err)
		}

		proposal := &matchmaking.Proposal{
			TeamA: [2]matchmaking.TeamAssignment{
				{PlayerID: in.TeamA[0].String(), Rating: ratings[in.TeamA[0]], Gender: genders[in.TeamA[0]]},
				{PlayerID: in.TeamA[1].String(), Rating: ratings[in.TeamA[1]], Gender: genders[in.TeamA[1]]},
			},
			TeamB: [2]matchmaking.TeamAssignment{
				{PlayerID: in.TeamB[0].String(), Rating: ratings[in.TeamB[0]], Gender: genders[in.TeamB[0]]},
				{PlayerID: in.TeamB[1].String(), Rating: ratings[in.TeamB[1]], Gender: genders[in.TeamB[1]]},
			},
		}
		if err := assignTeams(ctx, q, m.ID, proposal, byPlayer); err != nil {
			return err
		}

		if _, err := q.SetCourtStatus(ctx, db.SetCourtStatusParams{ID: court.ID, Status: db.CourtStatusPLAYING}); err != nil {
			return fmt.Errorf("set court playing: %w", err)
		}

		result = toMatch(m)
		result.Players = []uuid.UUID{
			in.TeamA[0], in.TeamA[1], in.TeamB[0], in.TeamB[1],
		}
		if s.outbox != nil {
			names := make(map[uuid.UUID]string, 4)
			for _, id := range ids {
				p, err := q.GetPlayer(ctx, id)
				if err != nil {
					return fmt.Errorf("get player name: %w", err)
				}
				names[id] = p.DisplayName
			}
			if err := s.outbox.Enqueue(ctx, tx, Announcement{SessionID: in.SessionID, MatchID: m.ID, CourtName: court.Name, Format: string(in.Format), TeamA: [2]string{names[in.TeamA[0]], names[in.TeamA[1]]}, TeamB: [2]string{names[in.TeamB[0]], names[in.TeamB[1]]}}); err != nil {
				return fmt.Errorf("enqueue group announcement: %w", err)
			}
		}
		return nil
	})

	if err != nil {
		if appErr, ok := apperr.As(err); ok {
			return Match{}, appErr
		}
		return Match{}, apperr.Internal(err)
	}

	s.logger.Info("match_generated", "session_id", in.SessionID, "match_id", result.ID, "format", in.Format, "mode", "manual")
	return result, nil
}

// validateFormatGenders enforces the single-gender formats. MIXED_DOUBLES
// is open to any gender mix, matching automatic matchmaking (see
// matchmaking.eligibleForFormat), so manual and automatic matches follow
// the same rule.
func validateFormatGenders(format Format, teamA, teamB [2]matchmaking.Gender) error {
	switch format {
	case MenDoubles:
		if teamA[0] != matchmaking.Male || teamA[1] != matchmaking.Male || teamB[0] != matchmaking.Male || teamB[1] != matchmaking.Male {
			return apperr.Validation("men's doubles requires all male players")
		}
	case WomenDoubles:
		if teamA[0] != matchmaking.Female || teamA[1] != matchmaking.Female || teamB[0] != matchmaking.Female || teamB[1] != matchmaking.Female {
			return apperr.Validation("women's doubles requires all female players")
		}
	}
	return nil
}

func requireDistinct(ids []uuid.UUID) error {
	seen := map[uuid.UUID]bool{}
	for _, id := range ids {
		if seen[id] {
			return apperr.Validation("duplicate player in match selection")
		}
		seen[id] = true
	}
	return nil
}

// mustSessionPlayerIDs resolves player IDs to session_player IDs so
// LockSessionPlayersByIDs (which locks by session_players.id) can be used.
// Any lookup failure here surfaces as a shorter locked-row count, which the
// caller already treats as PLAYER_NOT_ELIGIBLE.
func mustSessionPlayerIDs(ctx context.Context, q *db.Queries, sessionID uuid.UUID, playerIDs []uuid.UUID) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(playerIDs))
	for _, pid := range playerIDs {
		sp, err := q.GetSessionPlayerBySessionAndPlayer(ctx, db.GetSessionPlayerBySessionAndPlayerParams{
			SessionID: sessionID,
			PlayerID:  pid,
		})
		if err != nil {
			continue
		}
		out = append(out, sp.ID)
	}
	return out
}

type UpdateRosterInput struct {
	MatchID               uuid.UUID
	TeamA                 [2]uuid.UUID
	TeamB                 [2]uuid.UUID
	RemovedPlayerStatuses map[uuid.UUID]db.SessionPlayerStatus
}

// UpdateRoster atomically swaps, substitutes, or rearranges players in a match that is CREATED or PLAYING.
func (s *Service) UpdateRoster(ctx context.Context, in UpdateRosterInput) (Match, error) {
	results, err := s.UpdateRosters(ctx, []UpdateRosterInput{in})
	if err != nil {
		return Match{}, err
	}
	if len(results) == 0 {
		return Match{}, apperr.Internal(fmt.Errorf("no roster update result"))
	}
	return results[0], nil
}

// UpdateRosters atomically applies roster updates to one or more matches in the
// same transaction. This supports cross-court swaps where a player removed from
// one active match is added to another in the same batch.
func (s *Service) UpdateRosters(ctx context.Context, inputs []UpdateRosterInput) ([]Match, error) {
	if len(inputs) == 0 {
		return nil, nil
	}

	var results []Match
	err := database.RunInTx(ctx, s.pool, func(q *db.Queries) error {
		var err error
		results, err = s.updateRostersTx(ctx, q, inputs)
		return err
	})

	if err != nil {
		if appErr, ok := apperr.As(err); ok {
			return nil, appErr
		}
		return nil, apperr.Internal(err)
	}

	for _, m := range results {
		s.logger.Info("match_roster_updated", "match_id", m.ID)
	}
	return results, nil
}

func (s *Service) updateRostersTx(ctx context.Context, q *db.Queries, inputs []UpdateRosterInput) ([]Match, error) {
	if len(inputs) == 0 {
		return nil, nil
	}

	type planned struct {
		in          UpdateRosterInput
		match       db.Match
		existingMap map[uuid.UUID]db.MatchPlayer
		newMap      map[uuid.UUID]db.MatchTeam
	}

	results := make([]Match, len(inputs))
	plans := make([]planned, 0, len(inputs))
	seenMatches := map[uuid.UUID]bool{}
	movedAcross := map[uuid.UUID]bool{}

	// Lock matches and compute per-match plans. Also track players removed
	// in one match while being present in another update, allowing them to
	// stay PLAYING across a cross-court swap.
	for _, in := range inputs {
		ids := []uuid.UUID{in.TeamA[0], in.TeamA[1], in.TeamB[0], in.TeamB[1]}
		if err := requireDistinct(ids); err != nil {
			return nil, err
		}
		if seenMatches[in.MatchID] {
			return nil, apperr.Validation("duplicate match in roster update")
		}
		seenMatches[in.MatchID] = true

		m, err := q.LockMatchByID(ctx, in.MatchID)
		if err != nil {
			return nil, apperr.NotFound("match")
		}
		if m.Status == db.MatchStatusFINISHED {
			return nil, apperr.InvalidState("cannot modify roster of a finished match")
		}

		existingPlayers, err := q.ListMatchPlayers(ctx, in.MatchID)
		if err != nil {
			return nil, fmt.Errorf("list match players: %w", err)
		}
		existingMap := map[uuid.UUID]db.MatchPlayer{}
		for _, p := range existingPlayers {
			if p.PlayerID.Valid {
				existingMap[uuid.UUID(p.PlayerID.Bytes)] = p
			}
		}
		newMap := map[uuid.UUID]db.MatchTeam{
			in.TeamA[0]: db.MatchTeamA,
			in.TeamA[1]: db.MatchTeamA,
			in.TeamB[0]: db.MatchTeamB,
			in.TeamB[1]: db.MatchTeamB,
		}
		for pID := range existingMap {
			if _, kept := newMap[pID]; !kept {
				movedAcross[pID] = true
			}
		}

		plans = append(plans, planned{in: in, match: m, existingMap: existingMap, newMap: newMap})
	}

	// Only allow PLAYING-status additions when the player is removed from
	// another match in this same batch and added to another match in batch.
	for pID := range movedAcross {
		addedElsewhere := false
		for _, p := range plans {
			if _, was := p.existingMap[pID]; was {
				continue
			}
			if _, now := p.newMap[pID]; now {
				addedElsewhere = true
				break
			}
		}
		if !addedElsewhere {
			delete(movedAcross, pID)
		}
	}

	for i, p := range plans {
		in := p.in
		m := p.match
		existingMap := p.existingMap
		newMap := p.newMap
		ids := []uuid.UUID{in.TeamA[0], in.TeamA[1], in.TeamB[0], in.TeamB[1]}

		genders := map[uuid.UUID]matchmaking.Gender{}
		for _, id := range ids {
			pl, err := q.GetPlayer(ctx, id)
			if err != nil {
				return nil, fmt.Errorf("get player %s: %w", id, err)
			}
			genders[id] = matchmaking.Gender(pl.Gender)
		}
		if err := validateFormatGenders(Format(m.Format), [2]matchmaking.Gender{genders[in.TeamA[0]], genders[in.TeamA[1]]}, [2]matchmaking.Gender{genders[in.TeamB[0]], genders[in.TeamB[1]]}); err != nil {
			return nil, err
		}

		for pID := range existingMap {
			if _, kept := newMap[pID]; !kept {
				if err := q.DeleteMatchPlayer(ctx, db.DeleteMatchPlayerParams{MatchID: in.MatchID, PlayerID: pID}); err != nil {
					return nil, fmt.Errorf("delete match player: %w", err)
				}
				sp, err := q.GetSessionPlayerBySessionAndPlayerForUpdate(ctx, db.GetSessionPlayerBySessionAndPlayerForUpdateParams{SessionID: m.SessionID, PlayerID: pID})
				if err != nil {
					return nil, fmt.Errorf("get session player: %w", err)
				}
				nextStatus := db.SessionPlayerStatusBREAK
				if override, ok := in.RemovedPlayerStatuses[pID]; ok && string(override) != "" {
					nextStatus = override
				}
				// Cross-court swap: player moves to another match in this
				// same transaction, so keep PLAYING.
				if movedAcross[pID] {
					nextStatus = db.SessionPlayerStatusPLAYING
				}
				if sp.Status != nextStatus {
					if _, err := q.SetSessionPlayerStatus(ctx, db.SetSessionPlayerStatusParams{ID: sp.ID, Status: nextStatus}); err != nil {
						return nil, fmt.Errorf("update removed session player status: %w", err)
					}
				}
			}
		}

		for _, pID := range ids {
			team := newMap[pID]
			existingMP, kept := existingMap[pID]
			sp, err := q.GetSessionPlayerBySessionAndPlayerForUpdate(ctx, db.GetSessionPlayerBySessionAndPlayerForUpdateParams{SessionID: m.SessionID, PlayerID: pID})
			if err != nil {
				return nil, apperr.PlayerNotEligible("player is not part of this session")
			}
			if !kept {
				if sp.Status == db.SessionPlayerStatusPLAYING && !movedAcross[pID] {
					// Double check if player is legitimately playing in another active match
					count, err := q.CountActiveMatchesForPlayer(ctx, db.CountActiveMatchesForPlayerParams{
						PlayerID:  pgtype.UUID{Bytes: pID, Valid: true},
						SessionID: m.SessionID,
						ID:        in.MatchID,
					})
					if err == nil && count > 0 {
						return nil, apperr.Conflict("player is already playing in another match")
					}
				}
				if sp.Status == db.SessionPlayerStatusENDED {
					return nil, apperr.PlayerNotEligible("player has left the session")
				}
				r, err := q.GetPlayerRating(ctx, pID)
				if err != nil {
					return nil, fmt.Errorf("get player rating: %w", err)
				}
				if _, err := q.AddMatchPlayer(ctx, db.AddMatchPlayerParams{MatchID: in.MatchID, PlayerID: pID, Team: team, RatingBefore: pgtype.Float8{Float64: r.Rating, Valid: true}}); err != nil {
					return nil, fmt.Errorf("add match player: %w", err)
				}
				if sp.Status != db.SessionPlayerStatusPLAYING {
					if _, err := q.SetSessionPlayerStatus(ctx, db.SetSessionPlayerStatusParams{ID: sp.ID, Status: db.SessionPlayerStatusPLAYING}); err != nil {
						return nil, fmt.Errorf("set session player playing: %w", err)
					}
				}
			} else if existingMP.Team != team {
				if _, err := q.UpdateMatchPlayerTeam(ctx, db.UpdateMatchPlayerTeamParams{MatchID: in.MatchID, PlayerID: pID, Team: team}); err != nil {
					return nil, fmt.Errorf("update match player team: %w", err)
				}
			}
		}

		result := toMatch(m)
		result.Players = ids
		results[i] = result
	}

	return results, nil
}

type SwapAction string

const (
	SwapActionCourtToCourt SwapAction = "court_to_court"
	SwapActionSameCourt    SwapAction = "same_court"
	SwapActionCourtToBench SwapAction = "court_to_bench"
	SwapActionBothWaiting  SwapAction = "both_waiting"
)

type SwapPlayersInput struct {
	SessionID uuid.UUID
	Player1ID uuid.UUID
	Player2ID uuid.UUID
}

type SwapPlayersResult struct {
	Action  SwapAction `json:"action"`
	Message string     `json:"message"`
	Matches []Match    `json:"matches"`
}

// SwapPlayers atomically swaps, substitutes, or rearranges two players in a session:
// - Court <-> Court: swaps Player 1 and Player 2 across two active matches
// - Same Court: swaps Player 1 and Player 2 within the same match
// - Court <-> Waiting bench: substitutes a player on court with one waiting on bench (or vice versa)
// - Bench <-> Bench: reports both players are already waiting
func (s *Service) SwapPlayers(ctx context.Context, in SwapPlayersInput) (SwapPlayersResult, error) {
	if in.Player1ID == in.Player2ID {
		return SwapPlayersResult{}, apperr.Validation("choose two different players")
	}

	type loc struct {
		match db.Match
		teamA [2]uuid.UUID
		teamB [2]uuid.UUID
	}

	replaceIn := func(l *loc, out, inID uuid.UUID) ([2]uuid.UUID, [2]uuid.UUID) {
		a, b := l.teamA, l.teamB
		for i := 0; i < 2; i++ {
			if a[i] == out {
				a[i] = inID
			}
			if b[i] == out {
				b[i] = inID
			}
		}
		return a, b
	}

	swapInSame := func(l *loc, p1, p2 uuid.UUID) ([2]uuid.UUID, [2]uuid.UUID) {
		a, b := l.teamA, l.teamB
		swapID := func(x [2]uuid.UUID) [2]uuid.UUID {
			for i := 0; i < 2; i++ {
				if x[i] == p1 {
					x[i] = p2
				} else if x[i] == p2 {
					x[i] = p1
				}
			}
			return x
		}
		return swapID(a), swapID(b)
	}

	var res SwapPlayersResult
	err := database.RunInTx(ctx, s.pool, func(q *db.Queries) error {
		sp1, err := q.GetSessionPlayerBySessionAndPlayerForUpdate(ctx, db.GetSessionPlayerBySessionAndPlayerForUpdateParams{
			SessionID: in.SessionID,
			PlayerID:  in.Player1ID,
		})
		if err != nil {
			return apperr.PlayerNotEligible("player 1 is not part of this session")
		}
		sp2, err := q.GetSessionPlayerBySessionAndPlayerForUpdate(ctx, db.GetSessionPlayerBySessionAndPlayerForUpdateParams{
			SessionID: in.SessionID,
			PlayerID:  in.Player2ID,
		})
		if err != nil {
			return apperr.PlayerNotEligible("player 2 is not part of this session")
		}
		if sp1.Status == db.SessionPlayerStatusENDED || sp2.Status == db.SessionPlayerStatusENDED {
			return apperr.PlayerNotEligible("both players must be in the rotation (not ENDED)")
		}

		matches, err := q.ListMatchesBySession(ctx, in.SessionID)
		if err != nil {
			return fmt.Errorf("list matches: %w", err)
		}
		activeMatches := make(map[uuid.UUID]db.Match)
		for _, m := range matches {
			if m.Status == db.MatchStatusCREATED || m.Status == db.MatchStatusPLAYING {
				activeMatches[m.ID] = m
			}
		}

		allMPs, err := q.ListMatchPlayersBySession(ctx, in.SessionID)
		if err != nil {
			return fmt.Errorf("list session match players: %w", err)
		}

		locs := make(map[uuid.UUID]*loc)
		for _, mp := range allMPs {
			m, isActive := activeMatches[mp.MatchID]
			if !isActive || !mp.PlayerID.Valid {
				continue
			}
			pID := uuid.UUID(mp.PlayerID.Bytes)
			l, exists := locs[m.ID]
			if !exists {
				l = &loc{match: m}
				locs[m.ID] = l
			}
			if mp.Team == db.MatchTeamA {
				if l.teamA[0] == uuid.Nil {
					l.teamA[0] = pID
				} else {
					l.teamA[1] = pID
				}
			} else {
				if l.teamB[0] == uuid.Nil {
					l.teamB[0] = pID
				} else {
					l.teamB[1] = pID
				}
			}
		}

		var l1, l2 *loc
		for _, l := range locs {
			if l.teamA[0] == in.Player1ID || l.teamA[1] == in.Player1ID || l.teamB[0] == in.Player1ID || l.teamB[1] == in.Player1ID {
				l1 = l
			}
			if l.teamA[0] == in.Player2ID || l.teamA[1] == in.Player2ID || l.teamB[0] == in.Player2ID || l.teamB[1] == in.Player2ID {
				l2 = l
			}
		}

		// Look up names for user-friendly message
		pl1, _ := q.GetPlayer(ctx, in.Player1ID)
		pl2, _ := q.GetPlayer(ctx, in.Player2ID)
		name1 := pl1.DisplayName
		if name1 == "" {
			name1 = in.Player1ID.String()
		}
		name2 := pl2.DisplayName
		if name2 == "" {
			name2 = in.Player2ID.String()
		}

		var updateInputs []UpdateRosterInput

		switch {
		case l1 == nil && l2 == nil:
			// Both are on the bench. Self-heal any stale PLAYING status.
			if sp1.Status == db.SessionPlayerStatusPLAYING {
				_, _ = q.SetSessionPlayerStatus(ctx, db.SetSessionPlayerStatusParams{ID: sp1.ID, Status: db.SessionPlayerStatusWAITING})
			}
			if sp2.Status == db.SessionPlayerStatusPLAYING {
				_, _ = q.SetSessionPlayerStatus(ctx, db.SetSessionPlayerStatusParams{ID: sp2.ID, Status: db.SessionPlayerStatusWAITING})
			}
			res = SwapPlayersResult{
				Action:  SwapActionBothWaiting,
				Message: fmt.Sprintf("Both %s and %s are already waiting on the bench.", name1, name2),
			}
			return nil

		case l1 != nil && l2 == nil:
			// Player 1 on court, Player 2 on bench
			if sp2.Status == db.SessionPlayerStatusPLAYING {
				_, _ = q.SetSessionPlayerStatus(ctx, db.SetSessionPlayerStatusParams{ID: sp2.ID, Status: db.SessionPlayerStatusWAITING})
			}
			a, b := replaceIn(l1, in.Player1ID, in.Player2ID)
			updateInputs = []UpdateRosterInput{
				{
					MatchID:               l1.match.ID,
					TeamA:                 a,
					TeamB:                 b,
					RemovedPlayerStatuses: map[uuid.UUID]db.SessionPlayerStatus{in.Player1ID: db.SessionPlayerStatusWAITING},
				},
			}
			res.Action = SwapActionCourtToBench
			res.Message = fmt.Sprintf("%s is now playing instead of %s (back in queue).", name2, name1)

		case l1 == nil && l2 != nil:
			// Player 1 on bench, Player 2 on court
			if sp1.Status == db.SessionPlayerStatusPLAYING {
				_, _ = q.SetSessionPlayerStatus(ctx, db.SetSessionPlayerStatusParams{ID: sp1.ID, Status: db.SessionPlayerStatusWAITING})
			}
			a, b := replaceIn(l2, in.Player2ID, in.Player1ID)
			updateInputs = []UpdateRosterInput{
				{
					MatchID:               l2.match.ID,
					TeamA:                 a,
					TeamB:                 b,
					RemovedPlayerStatuses: map[uuid.UUID]db.SessionPlayerStatus{in.Player2ID: db.SessionPlayerStatusWAITING},
				},
			}
			res.Action = SwapActionCourtToBench
			res.Message = fmt.Sprintf("%s is now playing instead of %s (back in queue).", name1, name2)

		case l1.match.ID == l2.match.ID:
			// Same court
			a, b := swapInSame(l1, in.Player1ID, in.Player2ID)
			updateInputs = []UpdateRosterInput{
				{MatchID: l1.match.ID, TeamA: a, TeamB: b},
			}
			res.Action = SwapActionSameCourt
			res.Message = fmt.Sprintf("Swapped %s and %s on the same court.", name1, name2)

		default:
			// Court <-> Court
			a1, b1 := replaceIn(l1, in.Player1ID, in.Player2ID)
			a2, b2 := replaceIn(l2, in.Player2ID, in.Player1ID)
			updateInputs = []UpdateRosterInput{
				{MatchID: l1.match.ID, TeamA: a1, TeamB: b1},
				{MatchID: l2.match.ID, TeamA: a2, TeamB: b2},
			}
			res.Action = SwapActionCourtToCourt
			res.Message = fmt.Sprintf("Swapped %s and %s across courts.", name1, name2)
		}

		matchesUpdated, err := s.updateRostersTx(ctx, q, updateInputs)
		if err != nil {
			return err
		}
		res.Matches = matchesUpdated
		return nil
	})

	if err != nil {
		if appErr, ok := apperr.As(err); ok {
			return SwapPlayersResult{}, appErr
		}
		return SwapPlayersResult{}, apperr.Internal(err)
	}

	return res, nil
}

// ============================================================
// Start / Finish
// ============================================================

func (s *Service) StartMatch(ctx context.Context, matchID uuid.UUID) (Match, error) {
	var result Match
	err := database.RunInTx(ctx, s.pool, func(q *db.Queries) error {
		m, err := q.LockMatchByID(ctx, matchID)
		if err != nil {
			return apperr.NotFound("match")
		}
		if m.Status != db.MatchStatusCREATED {
			return apperr.InvalidState("match is not in CREATED state")
		}
		updated, err := q.StartMatch(ctx, matchID)
		if err != nil {
			return fmt.Errorf("start match: %w", err)
		}
		result = toMatch(updated)
		mp, _ := q.ListMatchPlayers(ctx, matchID)
		for _, p := range mp {
			if p.PlayerID.Valid {
				result.Players = append(result.Players, uuid.UUID(p.PlayerID.Bytes))
			}
		}
		return nil
	})
	if err != nil {
		if appErr, ok := apperr.As(err); ok {
			return Match{}, appErr
		}
		return Match{}, apperr.Internal(err)
	}
	s.logger.Info("match_started", "match_id", matchID)
	return result, nil
}

type FinishInput struct {
	MatchID uuid.UUID
	ScoreA  int32
	ScoreB  int32
}

// FinishMatch is the other concurrency/atomicity-critical transaction: match
// completion, winner determination, rating updates, rating history, and
// court release all happen together or not at all (spec section 18).
func (s *Service) FinishMatch(ctx context.Context, in FinishInput) (Match, error) {
	if in.ScoreA < 0 || in.ScoreB < 0 {
		return Match{}, apperr.Validation("scores must be non-negative")
	}
	if in.ScoreA == in.ScoreB {
		return Match{}, apperr.Validation("a match cannot end in a tie")
	}

	var result Match
	err := database.RunInTx(ctx, s.pool, func(q *db.Queries) error {
		m, err := q.LockMatchByID(ctx, in.MatchID)
		if err != nil {
			return apperr.NotFound("match")
		}
		if m.Status != db.MatchStatusPLAYING {
			return apperr.InvalidState("match is not in PLAYING state")
		}

		players, err := q.ListMatchPlayers(ctx, in.MatchID)
		if err != nil {
			return fmt.Errorf("list match players: %w", err)
		}
		if len(players) != 4 {
			return fmt.Errorf("expected 4 match players, got %d", len(players))
		}

		playerIDs := make([]uuid.UUID, 0, 4)
		for _, p := range players {
			playerIDs = append(playerIDs, matchPlayerID(p))
		}
		lockedRatings, err := q.LockPlayerRatingsByIDs(ctx, playerIDs)
		if err != nil {
			return fmt.Errorf("lock player ratings: %w", err)
		}
		ratingByPlayer := map[uuid.UUID]float64{}
		for _, r := range lockedRatings {
			ratingByPlayer[r.PlayerID] = r.Rating
		}

		var teamA, teamB [2]uuid.UUID
		ai, bi := 0, 0
		for _, p := range players {
			if p.Team == db.MatchTeamA {
				teamA[ai] = matchPlayerID(p)
				ai++
			} else {
				teamB[bi] = matchPlayerID(p)
				bi++
			}
		}

		teamAWon := in.ScoreA > in.ScoreB
		winner := db.MatchWinnerA
		if !teamAWon {
			winner = db.MatchWinnerB
		}

		mr := rating.MatchResult{
			TeamA: [2]rating.PlayerRef{
				{PlayerID: teamA[0].String(), Rating: ratingByPlayer[teamA[0]]},
				{PlayerID: teamA[1].String(), Rating: ratingByPlayer[teamA[1]]},
			},
			TeamB: [2]rating.PlayerRef{
				{PlayerID: teamB[0].String(), Rating: ratingByPlayer[teamB[0]]},
				{PlayerID: teamB[1].String(), Rating: ratingByPlayer[teamB[1]]},
			},
			TeamAWon: teamAWon,
		}
		results := rating.ApplyMatch(mr)

		for _, res := range results {
			playerID, err := uuid.Parse(res.PlayerID)
			if err != nil {
				return fmt.Errorf("parse player id: %w", err)
			}
			if _, err := q.UpdatePlayerRating(ctx, db.UpdatePlayerRatingParams{
				PlayerID: playerID,
				Rating:   res.RatingAfter,
			}); err != nil {
				return fmt.Errorf("update player rating: %w", err)
			}
			if _, err := q.InsertRatingHistory(ctx, db.InsertRatingHistoryParams{
				PlayerID:     playerID,
				MatchID:      in.MatchID,
				RatingBefore: res.RatingBefore,
				RatingAfter:  res.RatingAfter,
				RatingChange: res.RatingChange,
			}); err != nil {
				return fmt.Errorf("insert rating history: %w", err)
			}
			if _, err := q.SetMatchPlayerRatingAfter(ctx, db.SetMatchPlayerRatingAfterParams{
				MatchID:      in.MatchID,
				PlayerID:     playerID,
				RatingAfter:  pgtype.Float8{Float64: res.RatingAfter, Valid: true},
				RatingChange: pgtype.Float8{Float64: res.RatingChange, Valid: true},
			}); err != nil {
				return fmt.Errorf("set match player rating: %w", err)
			}

			sp, err := q.GetSessionPlayerBySessionAndPlayerForUpdate(ctx, db.GetSessionPlayerBySessionAndPlayerForUpdateParams{
				SessionID: m.SessionID,
				PlayerID:  playerID,
			})
			if err != nil {
				return fmt.Errorf("lock session player: %w", err)
			}
			won := int32(0)
			lost := int32(0)
			isTeamA := playerID == teamA[0] || playerID == teamA[1]
			if (isTeamA && teamAWon) || (!isTeamA && !teamAWon) {
				won = 1
			} else {
				lost = 1
			}
			if _, err := q.IncrementSessionPlayerMatchStats(ctx, db.IncrementSessionPlayerMatchStatsParams{
				ID:     sp.ID,
				Wins:   won,
				Losses: lost,
			}); err != nil {
				return fmt.Errorf("increment session player stats: %w", err)
			}
			// Default post-match state is WAITING; the player/host may move
			// to BREAK or ENDED afterward via the session-player status
			// endpoint (spec section 9).
			if _, err := q.SetSessionPlayerStatus(ctx, db.SetSessionPlayerStatusParams{
				ID:     sp.ID,
				Status: db.SessionPlayerStatusWAITING,
			}); err != nil {
				return fmt.Errorf("set session player waiting: %w", err)
			}
		}

		updated, err := q.FinishMatch(ctx, db.FinishMatchParams{
			ID:     in.MatchID,
			ScoreA: pgtype.Int4{Int32: in.ScoreA, Valid: true},
			ScoreB: pgtype.Int4{Int32: in.ScoreB, Valid: true},
			Winner: db.NullMatchWinner{MatchWinner: winner, Valid: true},
		})
		if err != nil {
			return fmt.Errorf("finish match: %w", err)
		}

		if _, err := q.SetCourtStatus(ctx, db.SetCourtStatusParams{ID: m.CourtID, Status: db.CourtStatusAVAILABLE}); err != nil {
			return fmt.Errorf("release court: %w", err)
		}

		result = toMatch(updated)
		for _, p := range players {
			if p.PlayerID.Valid {
				result.Players = append(result.Players, matchPlayerID(p))
			}
		}
		return nil
	})

	if err != nil {
		if appErr, ok := apperr.As(err); ok {
			return Match{}, appErr
		}
		return Match{}, apperr.Internal(err)
	}

	s.logger.Info("match_finished", "match_id", in.MatchID, "score_a", in.ScoreA, "score_b", in.ScoreB)
	s.logger.Info("rating_updated", "match_id", in.MatchID)
	return result, nil
}

// CancelMatch cancels an active or created match, releases the court, and restores
// the assigned players to WAITING status.
func (s *Service) CancelMatch(ctx context.Context, matchID uuid.UUID) error {
	err := database.RunInTx(ctx, s.pool, func(q *db.Queries) error {
		m, err := q.LockMatchByID(ctx, matchID)
		if err != nil {
			return apperr.NotFound("match")
		}
		if m.Status == db.MatchStatusFINISHED {
			return apperr.InvalidState("cannot cancel a finished match")
		}

		players, err := q.ListMatchPlayers(ctx, matchID)
		if err != nil {
			return fmt.Errorf("list match players: %w", err)
		}

		for _, p := range players {
			if p.PlayerID.Valid {
				pID := uuid.UUID(p.PlayerID.Bytes)
				sp, err := q.GetSessionPlayerBySessionAndPlayerForUpdate(ctx, db.GetSessionPlayerBySessionAndPlayerForUpdateParams{
					SessionID: m.SessionID,
					PlayerID:  pID,
				})
				if err == nil {
					if _, err := q.SetSessionPlayerStatus(ctx, db.SetSessionPlayerStatusParams{
						ID:     sp.ID,
						Status: db.SessionPlayerStatusWAITING,
					}); err != nil {
						return fmt.Errorf("restore session player waiting: %w", err)
					}
				}
			}
		}

		if _, err := q.SetCourtStatus(ctx, db.SetCourtStatusParams{ID: m.CourtID, Status: db.CourtStatusAVAILABLE}); err != nil {
			return fmt.Errorf("release court: %w", err)
		}

		if err := q.DeleteMatch(ctx, matchID); err != nil {
			return fmt.Errorf("delete match: %w", err)
		}
		return nil
	})

	if err != nil {
		if appErr, ok := apperr.As(err); ok {
			return appErr
		}
		return apperr.Internal(err)
	}

	s.logger.Info("match_cancelled", "match_id", matchID)
	return nil
}

func toProposal(format Format, p *matchmaking.Proposal) *Proposal {
	teamA := [2]uuid.UUID{uuid.MustParse(p.TeamA[0].PlayerID), uuid.MustParse(p.TeamA[1].PlayerID)}
	teamB := [2]uuid.UUID{uuid.MustParse(p.TeamB[0].PlayerID), uuid.MustParse(p.TeamB[1].PlayerID)}
	return &Proposal{
		Format:      format,
		TeamA:       teamA,
		TeamB:       teamB,
		TeamARating: p.TeamARating,
		TeamBRating: p.TeamBRating,
		RatingDiff:  p.RatingDiff,
	}
}
