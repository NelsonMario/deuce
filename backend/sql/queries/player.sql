-- name: CreatePlayer :one
INSERT INTO players (display_name, gender)
VALUES ($1, $2)
RETURNING *;

-- name: CreateGuestPlayer :one
INSERT INTO players (display_name, gender, is_guest)
VALUES ($1, $2, true)
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

-- name: DeleteStaleGuests :many
-- match_players.player_id and session_players.player_id both ON DELETE SET
-- NULL (see migration 000001), so deleting the guest here preserves the
-- match/session history it took part in (score, team, rating deltas,
-- session roster slot) and only clears the link back to its identity.
-- rating_history, player_ratings, club_members and player_tokens still
-- cascade away — those are the guest's own state/ledger, not match/session
-- history, and the guest is gone anyway.
DELETE FROM players
WHERE is_guest
  AND updated_at < sqlc.arg(cutoff)::timestamptz
  AND NOT EXISTS (
    SELECT 1
    FROM session_players sp
    JOIN sessions s ON s.id = sp.session_id
    WHERE sp.player_id = players.id
      AND s.status IN ('NOT_STARTED', 'ACTIVE')
  )
RETURNING id;

