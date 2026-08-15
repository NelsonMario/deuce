-- name: CreateClub :one
INSERT INTO clubs (name, host_player_id, join_code)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetClubByID :one
SELECT * FROM clubs WHERE id = $1;

-- name: GetClubByJoinCode :one
SELECT * FROM clubs WHERE join_code = $1;

-- name: AddClubMember :one
INSERT INTO club_members (club_id, player_id, role)
VALUES ($1, $2, $3)
ON CONFLICT (club_id, player_id) DO UPDATE SET role = club_members.role
RETURNING *;

-- name: GetClubMember :one
SELECT * FROM club_members WHERE club_id = $1 AND player_id = $2;

-- name: GetClubMemberPlayerByName :one
SELECT p.*
FROM players p
JOIN club_members cm ON cm.player_id = p.id
WHERE cm.club_id = $1 AND lower(p.display_name) = lower($2)
ORDER BY p.created_at
LIMIT 1;

-- name: ListClubMembers :many
SELECT * FROM club_members WHERE club_id = $1 ORDER BY joined_at;

-- name: UpdateClubMemberRole :one
UPDATE club_members SET role = $3
WHERE club_id = $1 AND player_id = $2
RETURNING *;
