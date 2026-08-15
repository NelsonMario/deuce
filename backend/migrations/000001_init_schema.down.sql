DROP TABLE IF EXISTS rating_history;
DROP TABLE IF EXISTS match_players;
DROP TABLE IF EXISTS matches;
DROP TABLE IF EXISTS courts;
DROP TABLE IF EXISTS session_players;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS player_ratings;
DROP TABLE IF EXISTS club_members;
DROP TABLE IF EXISTS clubs;
DROP TABLE IF EXISTS player_tokens;
DROP TABLE IF EXISTS players;

DROP TYPE IF EXISTS match_winner;
DROP TYPE IF EXISTS match_team;
DROP TYPE IF EXISTS match_status;
DROP TYPE IF EXISTS match_format;
DROP TYPE IF EXISTS court_status;
DROP TYPE IF EXISTS session_player_status;
DROP TYPE IF EXISTS session_assignment_mode;
DROP TYPE IF EXISTS session_status;
DROP TYPE IF EXISTS player_gender;
DROP TYPE IF EXISTS club_member_role;

DROP EXTENSION IF EXISTS pgcrypto;
