package match

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
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
type Service struct {
	pool   *pgxpool.Pool
	reads  ReadRepository
	locker lock.Locker
	logger *slog.Logger
}

func NewService(pool *pgxpool.Pool, reads ReadRepository, locker lock.Locker, logger *slog.Logger) *Service {
	return &Service{pool: pool, reads: reads, locker: locker, logger: logger}
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
// LockSessionPlayersByIDs). player_id is nullable at the schema level (see
// migration 000001) so a deleted guest's historical row can survive, but
// both call sites only ever select WAITING rows in an ACTIVE session — and
// the guest cleanup job never deletes a guest still in a NOT_STARTED/ACTIVE
// session (internal/player.CleanupStaleGuests) — so player_id is always
// non-null here.
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

	err := database.RunInTx(ctx, s.pool, func(q *db.Queries) error {
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
	err := database.RunInTx(ctx, s.pool, func(q *db.Queries) error {
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
		if in.Format == MixedDoubles {
			if !validMixedTeam(genders[in.TeamA[0]], genders[in.TeamA[1]]) || !validMixedTeam(genders[in.TeamB[0]], genders[in.TeamB[1]]) {
				return apperr.Validation("mixed doubles requires one male and one female per team")
			}
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

func validMixedTeam(a, b matchmaking.Gender) bool {
	return (a == matchmaking.Male && b == matchmaking.Female) || (a == matchmaking.Female && b == matchmaking.Male)
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
