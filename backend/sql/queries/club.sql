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

-- name: SetClubMemberGrade :one
UPDATE club_members SET grade = sqlc.narg(grade)
WHERE club_id = sqlc.arg(club_id) AND player_id = sqlc.arg(player_id)
RETURNING *;

-- name: ListClubRoster :many
-- Club members with their profile, host-assigned grade and current rating,
-- strongest first.
SELECT p.id AS player_id, p.display_name, p.gender, cm.role, cm.grade,
       COALESCE(pr.rating, 1000)::float8 AS rating
FROM club_members cm
JOIN players p ON p.id = cm.player_id
LEFT JOIN player_ratings pr ON pr.player_id = p.id
WHERE cm.club_id = $1
  -- The club's technical host identity (created with the club) is not a
  -- real player in the WhatsApp workflow.
  AND p.id <> (SELECT c.host_player_id FROM clubs c WHERE c.id = $1)
ORDER BY rating DESC, lower(p.display_name);

-- name: ListClubsByName :many
SELECT * FROM clubs WHERE lower(name) = lower($1) ORDER BY created_at;
