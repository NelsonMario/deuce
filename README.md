# deuce

Deuce is a WhatsApp-group service for running casual badminton sessions.
Configured group admins control the session; everyone in the group sees the
public roster, matches, and status. Fair rotation and rating-balanced doubles
remain automatic.

## What it does

- **One public group** — the bot only processes its configured WhatsApp
  group. It ignores private messages and other groups.
- **Admin controls** — configured WhatsApp JIDs create/start sessions, add
  players, configure courts, generate the next match, and finish a session.
- **Fair rotation** — players queue up (waiting/playing/break) and are
  cycled through courts so nobody sits out unfairly long.
- **Matchmaking** — matches are generated automatically (or built manually
  by the host) using player ratings and wait time to keep teams reasonably
  balanced. The host can flip between automatic and manual at any point
  mid-session, and turn fully-automatic court auto-fill on or off.
- **Ratings** — an Elo-style rating updates after every finished match, so
  the app gets better at balancing matches the more a club plays.
- **WhatsApp group bot (`whatsmeow`)** — one public group receives match
  assignments; anyone may read status while configured admin numbers manage
  the session. There are no player phone links or private messages.

## Structure

- `backend/` — Go API that owns all the club/session/matchmaking/rating
  logic. See `backend/README.md`.
- `frontend/` — a lightweight informational landing page. See
  `frontend/README.md`.
- `docker-compose.yml` — runs Postgres, migrations, and the backend
  together for local development.

## Running it locally

```bash
cp backend/.env.example .env   # set database and WhatsApp variables
docker compose up --build      # Postgres + migrations + backend on :8080
```

See [the backend guide](backend/README.md) for WhatsApp setup, group commands,
the data flow, and testing.

## Status

Actively evolving — see `TODO.md` for what's planned next and
`CHANGELOG.md` for what's shipped, by version.
