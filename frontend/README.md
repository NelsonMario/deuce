# Deuce landing page

This SvelteKit site introduces Deuce's one-group WhatsApp bot. The live
product is operated in WhatsApp, so this site intentionally has no player
login, club dashboard, queue, or match-management UI.

## Development

```bash
npm install
npm run dev
npm run check
npm run build
```

The site is a static build (`@sveltejs/adapter-static`, output in `build/`)
deployed on Vercel (`vercel.json`).

## How to use Deuce

Deuce runs inside **one WhatsApp group**. A bot account sits in the group as a
normal member, reads commands typed in that group, and posts court
assignments back to it. Players don't install anything or create accounts;
they only need to be in the group.

### Roles

| Who | Can do |
|---|---|
| Everyone in the group | Read the session: `help`, `status`, `players`, `matches` |
| Admins (phone numbers listed in `WHATSAPP_ADMIN_JIDS`) | Everything above, plus run the session: `start`, `stop`, `pause`, `resume`, `next`, `add`, `remove`, `courts`, `done`, `cancel` |

The bot ignores private chats and every other group. If a non-admin types an
admin command, the bot replies `⛔ This command is for group admins only.`

Normal conversation is safe. A command without arguments must be the whole
message, so `next` assigns a match but "next week?" does nothing, and
messages that aren't commands get no reply. Commands sent while the bot was
offline (older than two minutes) are skipped.

### Running a session (admin)

Type these as plain messages in the group:

1. **`start`**: creates a new automatic session if none exists, then starts
   it. If a session is already running it just says so.
2. **`courts 3`**: sets the number of courts (1–20). Courts can only be
   added, not removed.
3. **`add Alice F`**, **`add Bob M`**, …: adds a player to the waiting queue.
   The gender suffix `M`/`F` is optional and defaults to `M`. Names can have
   spaces: `add Mary Jane F`. A returning player keeps their rating.
4. Matches get assigned:
   - **Automatically**: auto-fill is on by default and keeps filling every
     free court using ratings and waiting time.
   - **On demand**: **`next`** assigns a match to the first free court right
     away.

   Either way, the bot announces the lineup (players, court, format) in the
   group.
5. **`done 1 21-15`**: when the game on Court 1 ends, report the score
   (Team A first, as in the announcement; `done 1 21 15` also works). Ratings
   update, the four players go back to the queue, and the court is free for
   the next match. Ties are rejected.
6. **`remove Bob`**: Bob went home, so take him out of the rotation. Players
   on court must finish first. `add Bob` brings him back.
7. **`pause`** / **`resume`**: turns auto-fill off or on, for example when
   you want to assign courts one at a time with `next`.
8. **`cancel`**: cancels the latest unfinished match; its players go back to
   the queue without a result.
9. **`stop`**: ends the session. Pending announcements for it are dropped.

### Checking in (everyone)

- **`status`**: session state, number of courts, players, playing, waiting
- **`players`**: roster with each player's status (waiting / playing / break / ended)
- **`matches`**: each match's court, status, and score
- **`help`**: the commands you can use (admins also see admin commands)

### Example

```text
Admin:  start
Bot:    🏸 Session created and started.
Admin:  courts 2
Bot:    ✅ 2 court(s) configured.
Admin:  add Alice F
Bot:    ✅ Alice added to the waiting queue.
…
Bot:    (lineup announcement for Court 1)
Admin:  done 1 21-15
Bot:    ✅ Court 1 finished 21–15. Ratings updated and the court is free.
Bot:    (next lineup announcement for Court 1)
Anyone: matches
Bot:    🏸 Matches
        • Court 1 — CREATED
        • Court 1 — FINISHED (21–15)
```

### Privacy note

Announcements contain player names, court, and format, and everyone in the
group can see them. Only use Deuce in a group where sharing that is fine.

### One-time bot setup (operator)

You do this once per club, on the backend:

1. Run the database migrations and create the club through the API
   (`POST /api/v1/clubs`). Note the club UUID.
2. Get a dedicated WhatsApp number for the bot (a prepaid SIM works well) and
   add it to the club group.
3. Set these in the repo-root `.env`:

   ```env
   WHATSAPP_GROUP_JID=1234567890-1234567890@g.us   # group ID, not its name
   WHATSAPP_ADMIN_JIDS=628123456789@s.whatsapp.net,628987654321@s.whatsapp.net
   WHATSAPP_CLUB_ID=<club UUID>
   ```

4. Start the backend, call `GET /api/v1/whatsapp/qr` (authenticated), and scan
   the code from the bot phone under **WhatsApp → Linked devices**. If it
   returns `WAITING`, a fresh code is being generated; call it again after a
   few seconds.
5. Confirm with `GET /api/v1/whatsapp/status`. The pairing is stored in
   Postgres and survives restarts.

If any `WHATSAPP_*` variable is missing, the bot stays disabled and the rest
of the API keeps working. To change who counts as an admin, edit
`WHATSAPP_ADMIN_JIDS` and restart.

For architecture, delivery guarantees, and the full API, see the
[backend README](../backend/README.md#whatsapp-bot-integration-whatsmeow).
