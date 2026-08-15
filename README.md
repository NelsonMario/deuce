# deuce

A casual badminton club app: create a club, share a join code, and run a
live session where players queue up, get assigned to courts, and play
matches — with fair rotation and rating-balanced doubles handled
automatically.

No accounts, no passwords. Players join a club/session with a code and get
a lightweight session token for the rest of the session. Returning on the
same device/browser is recognized automatically so ratings and history
carry over, instead of resetting on every join.

## What it does

- **Clubs & sessions** — a host creates a club, opens a session, and is
  added to that session as a waiting player automatically. Other players
  join via a code or QR link.
- **Fair rotation** — players queue up (waiting/playing/break) and are
  cycled through courts so nobody sits out unfairly long.
- **Matchmaking** — matches are generated automatically (or built manually
  by the host) using player ratings and wait time to keep teams reasonably
  balanced. The host can flip between automatic and manual at any point
  mid-session, and turn fully-automatic court auto-fill on or off.
- **Ratings** — an Elo-style rating updates after every finished match, so
  the app gets better at balancing matches the more a club plays.
- **Co-hosts** — a club can have more than one host, so session management
  isn't tied to a single device.

## Structure

- `backend/` — Go API that owns all the club/session/matchmaking/rating
  logic. See `backend/README.md`.
- `frontend/` — SvelteKit PWA that players and hosts actually use. See
  `frontend/README.md`.
- `docker-compose.yml` — runs Postgres, migrations, and the backend
  together for local development.

## Running it locally

```bash
cp backend/.env.example .env   # then edit PLAYER_TOKEN_SECRET
docker compose up --build      # Postgres + migrations + backend on :8080

cd frontend
npm install
npm run dev                    # frontend on http://localhost:5173
```

See the backend and frontend READMEs for configuration details, testing,
and how the two talk to each other.

## Status

Actively evolving — see `TODO.md` for what's planned next and
`CHANGELOG.md` for what's shipped, by version.
