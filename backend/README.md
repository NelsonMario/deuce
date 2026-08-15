# Badminton Club Backend — V1

Backend-first V1 for a badminton club app whose core value proposition is:

> Automatically rotate players fairly and create reasonably balanced doubles
> matches using player ratings, waiting time, and match history.

No frontend, no AI, no accounts. Players join via a club/session join code
and authenticate with an opaque bearer token for the rest of the session.

## Stack

| Concern         | Choice                                   |
|-----------------|-------------------------------------------|
| Language        | Go (module `deuce/backend`)              |
| HTTP            | Fiber v2                                   |
| Database access | `pgx` + `pgxpool` (no ORM)                 |
| SQL             | `sqlc` (`sql/queries` -> `internal/database/db`) |
| Migrations      | `golang-migrate` (`migrations/`)           |
| Config          | typed struct from env vars (`internal/config`) |
| Validation      | `go-playground/validator`                  |
| Logging         | `log/slog`, structured JSON                |
| API docs        | OpenAPI 3 (`docs/openapi.yaml`)            |
| Tests           | stdlib `testing` + `testcontainers-go`     |
| Container       | Docker / docker-compose                    |

## Architecture

```
HTTP (Fiber handlers)
  -> Application Service (per-module service.go)
    -> Domain logic (pure, in domain.go — rating & matchmaking have zero
       HTTP/DB dependencies and are unit tested in isolation)
      -> Repository interface (repository.go)
        -> PostgreSQL (pgx/sqlc)
```

Module layout:

```
internal/
  club/          club creation, membership, join codes
  player/        player identity, rating lookups
  session/       sessions, session-player rotation state, courts
  match/         match generation (auto/manual), lifecycle, concurrency-safe tx
  rating/        pure Elo-style engine (no HTTP/DB deps)
  matchmaking/   pure deterministic matchmaking algorithm (no HTTP/DB deps)
  auth/          token hashing, request principal
  apperr/        transport-agnostic application error type
  config/        typed env config
  database/      pgxpool bootstrap + transaction helper
  database/db/   sqlc-generated code (DO NOT EDIT)
  httpapi/       Fiber router, handlers, middleware
```

`rating` and `matchmaking` have no import of Fiber or pgx — they can be
fuzzed, benchmarked, or reused as a library without a database.

## Running locally

`docker-compose.yml` and `.env` live at the repo root (one level up from
`backend/`), so run these from there:

```bash
cp backend/.env.example .env        # then edit PLAYER_TOKEN_SECRET
docker compose up --build
```

This starts Postgres, runs migrations via the `migrate/migrate` image, and
starts the backend on `:8080`.

Optionally set `REDIS_HOST` (and `REDIS_PORT`/`REDIS_USERNAME`/
`REDIS_PASSWORD`/`REDIS_TLS`), e.g. to a Redis Cloud free-tier database, to
enable v2 device identity — see [Device identity (v2)](#device-identity-v2)
below. It's entirely optional: leave `REDIS_HOST` unset and the app runs
exactly as described everywhere else in this README.

To run without Docker (from `backend/`, with the repo-root `.env` exported
into your shell):

```bash
migrate -path migrations -database "postgres://$DB_USER:$DB_PASSWORD@$DB_HOST:$DB_PORT/$DB_NAME?sslmode=$DB_SSLMODE" up
go run ./cmd/server
```

## Testing

```bash
go test ./...
```

- `internal/rating` and `internal/matchmaking` are pure unit tests — no
  external dependencies.
- `internal/match` includes integration tests against a real Postgres
  instance via `testcontainers-go`. If Docker isn't available, they print
  `SKIP` and the suite still passes — they are not silently omitted from
  `go test ./...`, they just no-op.
- `TestIntegration_ConcurrentGeneration_NeverDoubleBooksAPlayer` fires two
  concurrent automatic match-generation requests at the same session and
  asserts every WAITING player is assigned to at most one match — the core
  concurrency guarantee from spec section 16.

## Concurrency & atomicity guarantees

1. **Match generation never double-books a player.** Every `WAITING`
   `session_player` row for a session is locked with `SELECT ... FOR UPDATE`
   before the matchmaking algorithm runs and before any row is flipped to
   `PLAYING`. A concurrent generation request for the same session blocks
   until the first transaction commits, then re-reads the now-updated
   statuses. See `internal/match/service.go: GenerateAutomatic`.
2. **Match finish and rating updates are atomic.** `FinishMatch` locks the
   match row, the four players' `player_ratings` rows, computes the Elo
   update, writes `rating_history`, updates `match_players`, updates the
   match, releases the court, and rotates the session players back to
   `WAITING` — all inside one transaction. A crash mid-way rolls back
   entirely; there is no state where a match is `FINISHED` but ratings are
   stale, or vice versa.
3. **Database constraints back up application logic**: enum types, foreign
   keys, uniqueness (`club_members(club_id, player_id)`,
   `session_players(session_id, player_id)`, `courts(session_id, name)`),
   and indexes on the hot paths (`session_players(session_id, status)`,
   `matches(session_id, status)`).

## Assignment modes & auto-fill

A session has an `assignment_mode` — `AUTOMATIC` (the engine picks the next
four players by rating + wait time) or `MANUAL` (the host picks the four and
the engine only suggests a balanced team split). It is set at creation but is
**not** fixed: `PATCH /sessions/:sessionId/assignment-mode` lets a host flip
it at any point in the session lifecycle — including while `ACTIVE` — and it
only affects how the *next* match is generated, never matches already in
progress.

On top of `AUTOMATIC` mode there is `auto_fill_enabled` (default `true`): a
background poller (`internal/match/autofill.go`, started in `cmd/server`)
keeps filling every empty `AVAILABLE` court with no per-court "Generate
match" trigger. A host can pause it with
`PATCH /sessions/:sessionId/auto-fill` without leaving `AUTOMATIC` mode, e.g.
to step in and hand-pick courts for a while. Auto-fill is meaningless in
`MANUAL` mode — the poller only scans `AUTOMATIC` sessions with
`auto_fill_enabled` set.

## API

See [`docs/openapi.yaml`](docs/openapi.yaml) for the full contract. Summary:

| Method | Path | Auth | Notes |
|---|---|---|---|
| POST | `/api/v1/clubs` | none | Host onboarding; creates host player + club |
| POST | `/api/v1/clubs/:clubId/join` | none | Join a club by join code |
| POST | `/api/v1/sessions` | host | Create a session |
| POST | `/api/v1/sessions/:sessionId/join` | none | QR-code join flow (spec §5) |
| GET | `/api/v1/sessions/:sessionId` | player | Session + players + courts |
| POST | `/api/v1/sessions/:sessionId/start` \| `/end` | host | |
| PATCH | `/api/v1/sessions/:sessionId/assignment-mode` | host | Toggle AUTOMATIC/MANUAL at any point, incl. mid-session |
| PATCH | `/api/v1/sessions/:sessionId/auto-fill` | host | Toggle fully-automatic court auto-fill on/off |
| POST | `/api/v1/sessions/:sessionId/courts` | host | |
| POST | `/api/v1/sessions/:sessionId/matches/generate` | host | AUTOMATIC mode |
| POST | `/api/v1/sessions/:sessionId/matches/manual/recommend` | host | MANUAL mode preview |
| POST | `/api/v1/sessions/:sessionId/matches/manual/confirm` | host | MANUAL mode confirm |
| GET | `/api/v1/sessions/:sessionId/matches` | player | |
| PATCH | `/api/v1/session-players/:id/status` | self or host | WAITING/BREAK/ENDED only |
| GET/POST | `/api/v1/matches/:matchId[/start\|/finish]` | player/host | |
| GET | `/api/v1/players/:playerId[/rating\|/matches]` | player | |

Errors always use:

```json
{ "error": { "code": "PLAYER_NOT_ELIGIBLE", "message": "..." } }
```

## Rate limiting

`middleware.RateLimit()` (`internal/httpapi/middleware/middleware.go`) applies
a 60 requests/minute budget, enabled by default (`internal/httpapi/router.go`).

Requests are keyed by bearer token (hashed the same way as for
authentication, so raw tokens are never held in Redis) when one is present,
so players sharing a venue's WiFi don't share one budget — the problem that
tripped the old IP-keyed version during multi-tab/multi-client testing.
Auth-free endpoints (club create/join, session join) fall back to keying by
`c.IP()`.

The budget itself is backed by `internal/ratelimit.RedisLimiter`, a fixed-
window counter on the same shared Redis connection as the device-identity
linker and the match-generation lock, so the limit is enforced across all
replicas instead of per-process. If Redis is unset or unreachable, it fails
open (`ratelimit.NoopLimiter` — no limiting) rather than blocking requests,
the same philosophy as every other optional Redis-backed feature in this
codebase.

## Authentication model

- **Club join code**: short, human-typeable, identifies a club — not a
  credential for a specific person.
- **Player session token**: 256-bit random value, returned once at join
  time, sent as `Authorization: Bearer <token>`. The server stores only an
  HMAC-SHA256 hash of it (keyed by `PLAYER_TOKEN_SECRET`); a database leak
  alone cannot be used to impersonate a player.
- Still no accounts, no passwords, no login. Rejoining without a recognized
  device (see below) always creates a new player identity.

## Device identity (v2)

An optional, lightweight alternative to full accounts: if `REDIS_HOST` is
set, a returning **device** (not a person, not an account — see
`internal/device`) can be recognized within a club, so its rating and match
history carry over instead of resetting at every join.

- The client generates a random `device_id` once (e.g. `crypto.randomUUID()`
  kept in `localStorage`) and sends it as an optional `device_id` field on
  `POST /clubs/`, `POST /clubs/:clubId/join`, and
  `POST /sessions/:sessionId/join`.
- The backend keeps a tiny Redis index: `device:<deviceID>:club:<clubID>` ->
  `<playerID>`, TTL ~180 days, refreshed on use. Postgres remains the source
  of truth for everything else — Redis only answers "have I seen this
  device in this club before, and as whom?"
- On a recognized rejoin: the existing player is reused (rating, match
  history, and club role — e.g. `HOST` — all carry over), its display
  name/gender are updated to whatever was just submitted, and a **fresh**
  bearer token is issued. The response's `PlayerAuthResponse.returning`
  field is `true` in this case, `false` for a brand-new identity. This is
  also how a host regains their host token from the same device/browser:
  hitting `/clubs/:clubId/join` again is recognized and returns their same
  (still-`HOST`) player.
- Identity recognition is scoped **per club, per device+browser** — the same
  physical device using a different browser, or joining a different club,
  starts fresh. There is no cross-device recovery; that would require real
  accounts (still out of scope, see below).
- Redis is best-effort and never required for correctness: if `REDIS_HOST` is
  unset, the config is invalid, or the instance is unreachable, this silently degrades to
  plain v1 behavior (every join is a new identity) rather than failing
  requests. Raw bearer tokens are still only ever hashed in Postgres, exactly
  as before — Redis never stores a token.

## Co-hosts

Every host-gated check (`club.Service.RequireHost`, and everything built on
it — sessions, courts, matches) authorizes by `club_members.role`, not by
the single `clubs.host_player_id` column (that column just records who
originally created the club). So multiple members can hold `role = HOST`
for the same club, and each gets full, independent host access — no
special-casing needed anywhere else.

- `GET /clubs/:clubId/members` (host only) lists members and roles.
- `POST /clubs/:clubId/members/:playerId/promote` (host only) grants an
  existing member HOST alongside the current host(s). The frontend presents
  this as "Make co-host". There's no "demote" endpoint.
- `GET /clubs/:clubId/me` lets any authenticated member self-check their
  own role — this is how a promoted co-host's own device recognizes its
  new status, since it has no local "I created this" signal the way the
  original creator's browser does.
- This is the real fix for "I lost/changed my host device" (see the device
  identity section above): promote a second device *before* you lose
  access to the first. Without that done in advance, there's no recovery
  path — the club's data stays intact, but nobody can act as its host.

## What's deliberately out of scope for V1

See spec section 33 / `docs/v2-plan.md` — no ML-based matchmaking, no
tournaments, no payments, no persistent accounts (login/password) or
cross-device identity recovery, no fairness-score analytics beyond raw
counters.
