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
cp .env.example .env              # set the DB_* (and optional REDIS_*) variables
docker compose run --rm migrate   # apply database migrations
docker compose up -d --build backend
```

Architecture, the data flow and testing are in the
[backend README](backend/README.md).

## Integrating the WhatsApp bot

You need a spare phone with its own WhatsApp number for the bot (don't use
your personal number), and two WhatsApp groups:

- **Players' group** (e.g. "Deuce Badminton"): players + the bot number.
  Only receives lineups and results.
- **Admin group** (e.g. "Deuce Admin"): you, co-hosts + the bot number.
  Everyone in it can run commands.

### Configuration

Everything about the bot (club, groups, admins) is set up from WhatsApp and
stored in the database. `.env` only holds infrastructure settings:

| Variable | Purpose |
|---|---|
| `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME` | Database connection (Supabase: session pooler details) |
| `DB_SSLMODE` | `require` for Supabase |
| `PLAYER_TOKEN_SECRET` | Secret for hashing API tokens (`openssl rand -hex 32`) |
| `REDIS_*` | Optional: shared rate limiting and locks. Leave `REDIS_HOST` unset to skip |
| `WHATSAPP_ENABLED` | Starts the bot (default `true`); `false` runs the API only |

Do not put phone numbers, group IDs or club IDs in `.env`. The old
`WHATSAPP_CLUB_ID`, `WHATSAPP_GROUP_JID` and `WHATSAPP_ADMIN_JIDS` variables
are no longer used; remove them.

### 1. Start the backend

```bash
docker compose run --rm migrate          # expect 1/u … 6/u (or "no change")
docker compose up -d --build backend
```

The bot starts automatically (`WHATSAPP_ENABLED=true` is the default). The
old `WHATSAPP_CLUB_ID` / `WHATSAPP_GROUP_JID` / `WHATSAPP_ADMIN_JIDS`
variables are not needed.

### 2. Get an API token

```bash
curl -s -X POST http://localhost:8080/api/v1/clubs -H 'Content-Type: application/json' \
  -d '{"club_name":"Tiger Club","host_display_name":"Host","host_gender":"MALE"}'
```

Copy `host.token` from the response:

```bash
TOKEN='<host.token>'
```

### 3. Pair the bot (once)

```bash
./scripts/whatsapp-qr.sh '<host.token>'
```

It opens the QR code (or says the bot is already paired). On the **bot
phone**: WhatsApp → Settings → Linked devices → Link a device, and scan.
Codes expire after about 20 seconds; if the scan fails, run the script
again. For a backend that isn't on localhost, pass its URL:
`./scripts/whatsapp-qr.sh '<host.token>' https://api.example.com`. The pairing is stored in
the database and survives restarts.

### 4. Set up the groups (on the bot phone)

| Group | Type this on the **bot phone** |
|---|---|
| Admin group | `setup admin Tiger Club` |
| Players' group | `setup announce` |

Use the same club name as in step 2 so the bot reuses that club.
`setup status` (from the bot phone) shows the current setup. Optional: in the
players' group settings, set *Send messages* to *Only admins* and make the bot
a group admin.

### Admins and co-hosts

Admin access is membership of the admin group; the bot stores no admin phone
numbers.

- **Add a co-host:** add them to the admin group in WhatsApp.
- **Remove a co-host:** remove them from the admin group (immediate).

The admin group is the access control, so lock it down in its **Group
settings**:

- **Add other members → Only admins**: only you (the WhatsApp group admin)
  decide who becomes a co-host.
- **Edit group settings → Only admins.**

Only the bot phone can run `setup`, so a co-host cannot move the bot to
another group.

### 5. Import the roster (in the admin group, from your phone)

```text
roster
Alice F A
Bob M B
Cara F B
Dan M C
```

One member per line: name, gender (M/F), grade (A strongest … E). Check with
`members`.

### 6. Test it

In the admin group: `start`, `courts 2`, `checkin Alice Bob Cara Dan`. The
lineup appears in the players' group. Finish with `done 1 21-15`, then
`stop` for the recap. Send `help` for all commands; the full command guide
is in [frontend/README.md](frontend/README.md#how-to-use-deuce).

### Troubleshooting

```bash
docker compose logs -f backend | grep whatsapp
```

| Log line | Fix |
|---|---|
| `whatsapp_command_received` → `whatsapp_reply_sent` | Working |
| `whatsapp_command_ignored reason=announcement_group` | Commands go in the admin group |
| `whatsapp_command_ignored reason=unconfigured_group` | Run `setup admin` in that group from the bot phone |
| `whatsapp_command_ignored reason=too_old` | Sent while the backend was down; send it again |

`GET /api/v1/whatsapp/status` (with the token) shows the connection and which
groups are set up.

### Resetting the database (e.g. for e2e tests)

Don't delete tables one by one: the enum types stay behind and the next
migration fails with `type "club_member_role" already exists`. Run this in
the Supabase SQL Editor instead:

```sql
DROP TABLE IF EXISTS
  whatsapp_bot_config, whatsapp_group_outbox, rating_history, match_players, matches,
  courts, session_players, sessions, player_ratings, club_members, clubs,
  player_tokens, players, schema_migrations
CASCADE;
DROP TYPE IF EXISTS
  club_member_role, player_gender, session_status, session_assignment_mode,
  session_player_status, court_status, match_format, match_status, match_team, match_winner
CASCADE;
```

Then repeat steps 1, 2, 4 and 5. The bot stays paired (its `whatsmeow`
schema is kept).

## Status

Actively evolving — see `TODO.md` for what's planned next and
`CHANGELOG.md` for what's shipped, by version.
