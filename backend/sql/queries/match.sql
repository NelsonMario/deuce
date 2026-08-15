-- name: CreateMatch :one
INSERT INTO matches (session_id, court_id, format)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetMatchByID :one
SELECT * FROM matches WHERE id = $1;

-- name: LockMatchByID :one
SELECT * FROM matches WHERE id = $1 FOR UPDATE;

-- name: StartMatch :one
UPDATE matches
SET status = 'PLAYING', started_at = now(), updated_at = now()
WHERE id = $1
RETURNING *;

-- name: FinishMatch :one
UPDATE matches
SET status = 'FINISHED', ended_at = now(), score_a = $2, score_b = $3, winner = $4, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: ListMatchesBySession :many
SELECT * FROM matches WHERE session_id = $1 ORDER BY created_at DESC;

-- name: AddMatchPlayer :one
-- player_id is cast explicitly because the column is nullable (see
-- migration 000001 — cleared via ON DELETE SET NULL when a guest is later
-- deleted); a live match is always assigned a real player, so keep the
-- parameter typed as a plain, non-nullable uuid.
INSERT INTO match_players (match_id, player_id, team, rating_before)
VALUES (sqlc.arg(match_id), sqlc.arg(player_id)::uuid, sqlc.arg(team), sqlc.arg(rating_before))
RETURNING *;

-- name: ListMatchPlayers :many
SELECT * FROM match_players WHERE match_id = $1;

-- name: SetMatchPlayerRatingAfter :one
UPDATE match_players
SET rating_after = sqlc.arg(rating_after), rating_change = sqlc.arg(rating_change)
WHERE match_id = sqlc.arg(match_id) AND player_id = sqlc.arg(player_id)::uuid
RETURNING *;
