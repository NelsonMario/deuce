-- name: CreatePlayer :one
INSERT INTO players (display_name, gender)
VALUES ($1, $2)
RETURNING *;

-- name: GetPlayer :one
SELECT * FROM players WHERE id = $1;

-- name: UpdatePlayerProfile :one
UPDATE players
SET display_name = $2, gender = $3, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: CreatePlayerToken :one
INSERT INTO player_tokens (player_id, token_hash)
VALUES ($1, $2)
RETURNING *;

-- name: GetPlayerByTokenHash :one
SELECT p.*
FROM players p
JOIN player_tokens t ON t.player_id = p.id
WHERE t.token_hash = $1 AND t.revoked_at IS NULL;

-- name: CreatePlayerRating :one
INSERT INTO player_ratings (player_id, rating)
VALUES ($1, $2)
RETURNING *;

-- name: GetPlayerRating :one
SELECT * FROM player_ratings WHERE player_id = $1;

-- name: LockPlayerRatingsByIDs :many
SELECT * FROM player_ratings
WHERE player_id = ANY(sqlc.arg(player_ids)::uuid[])
ORDER BY player_id
FOR UPDATE;

-- name: UpdatePlayerRating :one
UPDATE player_ratings
SET rating = $2, updated_at = now()
WHERE player_id = $1
RETURNING *;

-- name: InsertRatingHistory :one
INSERT INTO rating_history (player_id, match_id, rating_before, rating_after, rating_change)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListRatingHistoryByPlayer :many
SELECT * FROM rating_history
WHERE player_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListMatchesByPlayer :many
-- player_id is cast explicitly because match_players.player_id is nullable
-- (see migration 000001); listing a specific player's matches always looks
-- up a real player, so keep the parameter typed as a plain, non-nullable
-- uuid.
SELECT m.*
FROM matches m
JOIN match_players mp ON mp.match_id = m.id
WHERE mp.player_id = sqlc.arg(player_id)::uuid
ORDER BY m.created_at DESC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: GetPlayersByIDs :many
SELECT * FROM players
WHERE id = ANY(sqlc.arg(player_ids)::uuid[]);

-- name: GetPlayerRatingsByIDs :many
SELECT * FROM player_ratings
WHERE player_id = ANY(sqlc.arg(player_ids)::uuid[]);

