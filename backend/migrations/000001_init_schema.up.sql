-- Extensions
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ============================================================
-- Enums
-- ============================================================
CREATE TYPE club_member_role AS ENUM ('HOST', 'PLAYER');
CREATE TYPE player_gender AS ENUM ('MALE', 'FEMALE');
CREATE TYPE session_status AS ENUM ('NOT_STARTED', 'ACTIVE', 'FINISHED');
CREATE TYPE session_assignment_mode AS ENUM ('AUTOMATIC', 'MANUAL');
CREATE TYPE session_player_status AS ENUM ('WAITING', 'PLAYING', 'BREAK', 'ENDED');
CREATE TYPE court_status AS ENUM ('AVAILABLE', 'PLAYING');
CREATE TYPE match_format AS ENUM ('MIXED_DOUBLES', 'MEN_DOUBLES', 'WOMEN_DOUBLES');
CREATE TYPE match_status AS ENUM ('CREATED', 'PLAYING', 'FINISHED');
CREATE TYPE match_team AS ENUM ('A', 'B');
CREATE TYPE match_winner AS ENUM ('A', 'B');

-- ============================================================
-- Players (global identity)
-- ============================================================
CREATE TABLE players (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    display_name text NOT NULL CHECK (length(trim(display_name)) > 0),
    gender       player_gender NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);

-- Player session tokens (accountless auth). Never store raw tokens.
CREATE TABLE player_tokens (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    player_id  uuid NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    token_hash text NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz
);
CREATE INDEX idx_player_tokens_player_id ON player_tokens(player_id);

-- ============================================================
-- Clubs
-- ============================================================
CREATE TABLE clubs (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name           text NOT NULL CHECK (length(trim(name)) > 0),
    host_player_id uuid NOT NULL REFERENCES players(id),
    join_code      text NOT NULL UNIQUE,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE club_members (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    club_id    uuid NOT NULL REFERENCES clubs(id) ON DELETE CASCADE,
    player_id  uuid NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    role       club_member_role NOT NULL DEFAULT 'PLAYER',
    joined_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (club_id, player_id)
);
CREATE INDEX idx_club_members_club_player ON club_members(club_id, player_id);

-- ============================================================
-- Player rating (current) + history
-- ============================================================
CREATE TABLE player_ratings (
    player_id  uuid PRIMARY KEY REFERENCES players(id) ON DELETE CASCADE,
    rating     double precision NOT NULL DEFAULT 1000,
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_player_ratings_player_id ON player_ratings(player_id);

-- ============================================================
-- Sessions
-- ============================================================
CREATE TABLE sessions (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    club_id         uuid NOT NULL REFERENCES clubs(id) ON DELETE CASCADE,
    name            text NOT NULL DEFAULT '',
    status          session_status NOT NULL DEFAULT 'NOT_STARTED',
    assignment_mode session_assignment_mode NOT NULL DEFAULT 'AUTOMATIC',
    started_at      timestamptz,
    ended_at        timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_sessions_club_id ON sessions(club_id);

CREATE TABLE session_players (
    id                          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id                  uuid NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    player_id                   uuid NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    status                      session_player_status NOT NULL DEFAULT 'WAITING',
    waiting_started_at          timestamptz NOT NULL DEFAULT now(),
    accumulated_waiting_seconds bigint NOT NULL DEFAULT 0,
    matches_played              integer NOT NULL DEFAULT 0,
    wins                        integer NOT NULL DEFAULT 0,
    losses                      integer NOT NULL DEFAULT 0,
    created_at                  timestamptz NOT NULL DEFAULT now(),
    updated_at                  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (session_id, player_id)
);
CREATE INDEX idx_session_players_session_status ON session_players(session_id, status);
CREATE INDEX idx_session_players_player_id ON session_players(player_id);

CREATE TABLE courts (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id uuid NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    name       text NOT NULL,
    status     court_status NOT NULL DEFAULT 'AVAILABLE',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (session_id, name)
);
CREATE INDEX idx_courts_session_id ON courts(session_id);

-- ============================================================
-- Matches
-- ============================================================
CREATE TABLE matches (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id uuid NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    court_id   uuid NOT NULL REFERENCES courts(id),
    format     match_format NOT NULL,
    status     match_status NOT NULL DEFAULT 'CREATED',
    started_at timestamptz,
    ended_at   timestamptz,
    score_a    integer,
    score_b    integer,
    winner     match_winner,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (score_a IS NULL OR score_a >= 0),
    CHECK (score_b IS NULL OR score_b >= 0)
);
CREATE INDEX idx_matches_session_status ON matches(session_id, status);
CREATE INDEX idx_matches_court_id ON matches(court_id);

CREATE TABLE match_players (
    match_id      uuid NOT NULL REFERENCES matches(id) ON DELETE CASCADE,
    player_id     uuid NOT NULL REFERENCES players(id),
    team          match_team NOT NULL,
    rating_before double precision,
    rating_after  double precision,
    rating_change double precision,
    PRIMARY KEY (match_id, player_id)
);
CREATE INDEX idx_match_players_match_id ON match_players(match_id);
CREATE INDEX idx_match_players_player_id ON match_players(player_id);

-- NOTE: "a player can never be in two active matches" is NOT enforceable via a
-- plain partial unique index (Postgres forbids subqueries in index predicates).
-- It is instead guaranteed by the match-generation transaction, which locks the
-- candidate session_players rows FOR UPDATE and re-checks status == WAITING
-- immediately before flipping them to PLAYING inside the same transaction.

CREATE TABLE rating_history (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    player_id     uuid NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    match_id      uuid NOT NULL REFERENCES matches(id) ON DELETE CASCADE,
    rating_before double precision NOT NULL,
    rating_after  double precision NOT NULL,
    rating_change double precision NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_rating_history_player_id ON rating_history(player_id);
CREATE INDEX idx_rating_history_match_id ON rating_history(match_id);
