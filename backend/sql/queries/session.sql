-- name: CreateSession :one
INSERT INTO sessions (club_id, name, assignment_mode)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetSessionByID :one
SELECT * FROM sessions WHERE id = $1;

-- name: LockSessionByID :one
SELECT * FROM sessions WHERE id = $1 FOR UPDATE;

-- name: StartSession :one
UPDATE sessions
SET status = 'ACTIVE', started_at = now(), updated_at = now()
WHERE id = $1
RETURNING *;

-- name: EndSession :one
UPDATE sessions
SET status = 'FINISHED', ended_at = now(), updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UpdateSessionAssignmentMode :one
UPDATE sessions
SET assignment_mode = $2, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UpdateSessionAutoFillEnabled :one
UPDATE sessions
SET auto_fill_enabled = $2, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: ListSessionsByClub :many
SELECT * FROM sessions WHERE club_id = $1 ORDER BY created_at DESC;

-- name: ListActiveAutoFillSessions :many
-- Sessions eligible for the fully-automatic auto-fill background job: ACTIVE,
-- AUTOMATIC assignment, and auto_fill_enabled. Read-only (no locking) — the
-- job's per-session coordination lock and match.Service.GenerateAutomatic's
-- own transactional locking handle correctness.
SELECT * FROM sessions
WHERE status = 'ACTIVE' AND assignment_mode = 'AUTOMATIC' AND auto_fill_enabled = true
ORDER BY created_at;

-- ============================================================
-- Session players
-- ============================================================

-- name: AddSessionPlayer :one
-- player_id is cast explicitly because the column is nullable (see
-- migration 000001 — cleared via ON DELETE SET NULL when a guest is later
-- deleted); joining a session always assigns a real player, so keep the
-- parameter typed as a plain, non-nullable uuid.
INSERT INTO session_players (session_id, player_id)
VALUES (sqlc.arg(session_id), sqlc.arg(player_id)::uuid)
ON CONFLICT (session_id, player_id) DO UPDATE SET session_id = session_players.session_id
RETURNING *;

-- name: GetSessionPlayer :one
SELECT * FROM session_players WHERE id = $1;

-- name: GetSessionPlayerBySessionAndPlayer :one
SELECT * FROM session_players WHERE session_id = sqlc.arg(session_id) AND player_id = sqlc.arg(player_id)::uuid;

-- name: GetSessionPlayerBySessionAndPlayerForUpdate :one
SELECT * FROM session_players WHERE session_id = sqlc.arg(session_id) AND player_id = sqlc.arg(player_id)::uuid FOR UPDATE;

-- name: ListSessionPlayers :many
SELECT * FROM session_players WHERE session_id = $1 ORDER BY created_at;

-- name: ListWaitingSessionPlayersForUpdate :many
-- Eligible players for matchmaking, locked to serialize concurrent match generation.
SELECT * FROM session_players
WHERE session_id = $1 AND status = 'WAITING'
ORDER BY player_id
FOR UPDATE;

-- name: LockSessionPlayersByIDs :many
SELECT * FROM session_players
WHERE id = ANY(sqlc.arg(ids)::uuid[])
ORDER BY id
FOR UPDATE;

-- name: SetSessionPlayerStatus :one
UPDATE session_players
SET status = $2,
    waiting_started_at = CASE WHEN $2::session_player_status = 'WAITING' THEN now() ELSE waiting_started_at END,
    accumulated_waiting_seconds = CASE
        WHEN status = 'WAITING' AND $2::session_player_status != 'WAITING'
            THEN accumulated_waiting_seconds + GREATEST(0, EXTRACT(EPOCH FROM (now() - waiting_started_at))::bigint)
        ELSE accumulated_waiting_seconds
    END,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: IncrementSessionPlayerMatchStats :one
UPDATE session_players
SET matches_played = matches_played + 1,
    wins = wins + $2,
    losses = losses + $3,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- ============================================================
-- Courts
-- ============================================================

-- name: CreateCourt :one
INSERT INTO courts (session_id, name)
VALUES ($1, $2)
RETURNING *;

-- name: ListCourtsBySession :many
SELECT * FROM courts WHERE session_id = $1 ORDER BY name;

-- name: GetCourtByID :one
SELECT * FROM courts WHERE id = $1;

-- name: LockAvailableCourtForUpdate :one
SELECT * FROM courts
WHERE id = $1 AND status = 'AVAILABLE'
FOR UPDATE;

-- name: LockCourtByID :one
SELECT * FROM courts WHERE id = $1 FOR UPDATE;

-- name: ListAvailableCourtsBySession :many
SELECT * FROM courts WHERE session_id = $1 AND status = 'AVAILABLE' ORDER BY name;

-- name: SetCourtStatus :one
UPDATE courts SET status = $2, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: CountWaitingSessionPlayers :one
-- Read-only headcount used by the auto-fill background job to decide how
-- many of a session's AVAILABLE courts it can fill this tick; the
-- authoritative check happens inside GenerateAutomatic's own FOR UPDATE
-- transaction.
SELECT count(*) FROM session_players WHERE session_id = $1 AND status = 'WAITING';
