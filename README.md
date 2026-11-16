# deuce

Deuce is a WhatsApp bot for running casual badminton sessions. Hosts run
the session from a small admin group; players follow lineups and results in
the club's announcement group. Fair rotation and rating-balanced doubles are
automatic, and hosts can still pick matches by hand.

## What it does

- **Two groups, no private messages:** hosts type commands in an admin group
  (everyone in it is an admin); the players' group only receives lineups,
  results and session start/end. The bot ignores private chats.
- **Set up from WhatsApp:** `setup admin` / `setup announce`, typed on the
  bot's phone, configure the bot. No `.env` edits or restarts.
- **Fair rotation:** players queue up and are cycled through courts so nobody
  sits out unfairly long.
- **Matchmaking:** automatic (ratings + waiting time) or manual
  (`suggest` a balanced split, `assign` an exact lineup), switchable at any
  time.
- **Roster and grades:** import the club roster in one message with A–E
  grades that set starting ratings; check members in to a session by name.
- **Ratings and recap:** an Elo-style rating updates after every match, and
  `stop` sends hosts a recap of games, wins/losses, rating changes and
  suggested re-grades.

## Structure

- `backend/` — Go API that owns all the club/session/matchmaking/rating
  logic. See `backend/README.md`.
- `frontend/` — a lightweight informational landing page. See
  `frontend/README.md`.
- `docker-compose.yml` — runs Postgres, migrations, and the backend
  together for local development.

## Running it locally

```bash
cp .env.example .env           # set database (and optional Redis) variables
docker compose up --build      # Postgres + migrations + backend on :8080
```

Then follow the [setup guide](frontend/README.md#how-to-use-deuce) to pair
the bot and set up the groups. Architecture, the data flow and testing are in
the [backend README](backend/README.md).

## Status

Actively evolving — see `TODO.md` for what's planned next and
`CHANGELOG.md` for what's shipped, by version.
