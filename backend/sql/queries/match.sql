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
INSERT INTO match_players (match_id, player_id, team, rating_before)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListMatchPlayers :many
SELECT * FROM match_players WHERE match_id = $1;

-- name: SetMatchPlayerRatingAfter :one
UPDATE match_players
SET rating_after = $3, rating_change = $4
WHERE match_id = $1 AND player_id = $2
RETURNING *;
