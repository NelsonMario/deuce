# Deuce landing page

This SvelteKit site is the documentation page for the Deuce WhatsApp bot:
configuration, setup, admins and co-hosts, commands, grades and
troubleshooting (`src/routes/+page.svelte`). The bot itself is operated in
WhatsApp, so the site has no login, dashboard or match-management UI.

## Development

```bash
cp .env.example .env   # optional: set VITE_API_BASE_URL
npm install
npm run dev
npm run check
npm run build
```

Set `VITE_API_BASE_URL` to your backend origin (e.g. `http://localhost:8080`); a trailing `/api/v1` is also accepted. The bot setup page hides the backend URL by default; enable “Use custom API base URL” only when you need to override it. If unset, dev defaults to `http://localhost:8080` and production defaults to the current site origin.

The site is a static build (`@sveltejs/adapter-static`, output in `build/`)
deployed on Vercel (`vercel.json`).

## How to use Deuce

Deuce is a WhatsApp bot that runs your badminton session. It uses **two
groups**:

```text
"Deuce Admin"  (you + co-hosts + bot)           "Deuce Badminton"  (players + bot)
──────────────────────────────────────          ──────────────────────────────────
You:  in Alice Bob Cara Dan                     🏸 Session started!
Bot:  ✅ Checked in (4): Alice, Bob, …          🏸 Court 1: Alice / Bob vs Cara / Dan
You:  done 1 21-15                              ✅ Court 1 result
Bot:  ✅ Court 1 finished 21–15.                   Alice / Bob beat Cara / Dan 21–15
```

- **Admin group:** hosts type commands here. Everyone in this group is an
  admin; add or remove co-hosts in WhatsApp's group settings.
- **Players' group:** only receives session start/end, lineups and results.
  The bot ignores everything typed here, so chat and typos never trigger it.
- The bot never sends private messages and ignores private chats.

### What you need

- A **separate WhatsApp number for the bot** (a spare phone with a prepaid
  SIM works well). Don't use your personal number: the bot posts as that
  account, and commands typed from the bot's own number are treated as setup
  commands.
- The backend running (`docker compose up -d backend`) with migrations
  applied (`docker compose run --rm migrate`).

### One-time setup

1. **Create the groups.** Add the bot's number to your players' group, and
   create a small admin group with you, any co-hosts and the bot.
2. **Pair the bot** (only once; the pairing survives restarts). You need a
   Deuce token: create a club through the API and keep `host.token` from the
   response:

   ```bash
   curl -s -X POST http://localhost:8080/api/v1/clubs -H 'Content-Type: application/json' \
     -d '{"club_name":"Tiger Club","host_display_name":"Host","host_gender":"MALE"}'
   ```

   Then open the QR code and scan it on the bot phone under
   **WhatsApp → Settings → Linked devices → Link a device** (codes change
   about every 20 seconds; run it again if the scan fails):

   ```bash
   TOKEN='<host token>'
   CODE=$(curl -s -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/v1/whatsapp/qr \
     | python3 -c 'import json,sys; print(json.load(sys.stdin).get("qr",""))')
   npx -y qrcode@1.5.4 -o /tmp/whatsapp-qr.png "$CODE" && open /tmp/whatsapp-qr.png
   ```

3. **Set up the admin group.** On the **bot's phone**, open the admin group
   and send:

   ```text
   setup admin Tiger Club
   ```

   The bot replies that this is now the admin group. Use the same club name
   as in step 2 to reuse that club; a new name creates a new club.
4. **Set up the players' group.** On the **bot's phone**, open the players'
   group and send `setup announce`.
5. **Optional: lock the players' group.** In the group settings, set "Send
   messages" to *Only admins* and make the bot a group admin, so the group
   stays purely announcements. The bot must be a group admin to post there.
6. **Add your members** (from your own phone, in the admin group). One
   member per line: name, gender (M/F) and grade (A strongest … E):

   ```text
   member
   Alice F A
   Bob M B
   Cara F B
   Mary Jane F C
   ```

`setup status` (sent from the bot's phone) shows the current setup. Running
`setup admin` or `setup announce` in another group moves that role there.

### Running a session

All commands go in the **admin group**. Commands without extra words must be
the whole message (`next`, not "next please").

1. `start`: starts a session. Players see "Session started".
2. `courts 2`: number of courts (courts can only be added).
3. `in Alice Bob Cara Dan Eve`: checks members in (use commas for names
   with spaces: `in Mary Jane, Bob`). For someone new, add their gender and
   grade: `in Zed M C`; they become a member too. A name without gender or
   grade that isn't a member is reported, never created, so typos are safe.
4. Matches are assigned **automatically** to free courts, balancing ratings
   and waiting time. Each lineup is posted to the players' group.
5. When a game ends: `done 1 21-15` (court, then Team A's score first, as in
   the lineup). Ratings update, the result is posted, and the court gets the
   next match.
6. Someone leaves: `out Bob`. They can come back with `in Bob`.
7. `stop`: ends the session. Players see "Session finished"; you get the
   recap.

**Picking matches yourself:**

- `manual`: you pick lineups (`auto` lets the bot fill courts again).
- `suggest Alice Bob Cara Dan`: shows the most balanced split.
- `assign 1 Alice Bob vs Cara Dan`: puts that lineup on Court 1.
- `next`: fills one free court automatically.
- `cancel`: undoes the latest match. `cancel 2` undoes the match on Court 2.
- `set Alice Bob`: swaps players — bench↔court or court↔court (e.g. sub someone in, or trade spots).

### Commands (quick guide)

| Command | What it does | Example |
|---|---|---|
| `chat <message>` | Talk naturally; bot turns it into commands | `chat court 1 finished 21-15` |
| `start` / `stop` | Start or end the session | `start` |
| `status` | See courts, waiting list, recent results | `status` |
| `in <names>` | Check players in (new: add gender+grade) | `in Alice, Bob` / `in Eko M C` |
| `out <name>` | Remove a player from rotation | `out Bob` |
| `courts <n>` | Set number of courts | `courts 2` |
| `next` | Fill one free court | `next` |
| `done <court> <a>-<b>` | Save result, free court (Team A first) | `done 1 21-15` |
| `cancel [court]` | Undo a match (latest, or a specific court) | `cancel` / `cancel 2` |
| `set <name1> <name2>` | Swap players: bench↔court or court↔court | `set Alice Bob` |
| `auto` / `manual` | Bot fills courts / you pick lineups | `manual` |
| `suggest <4 names>` | Get the most balanced split | `suggest Alice Bob Cara Dan` |
| `assign <court> A B vs C D` | Set an exact lineup | `assign 1 Alice Bob vs Cara Dan` |
| `member` / `member <name>` | List members or view one | `member Alice` |
| `grade <name> <A-E>` | Change grade and reset rating | `grade Bob A` |
| `help` | Show command list | `help` |

Tip: names with spaces use commas, e.g. `in Mary Jane, Bob`. Setup commands (`setup admin/announce/status`) are run on the bot’s phone only. Old command names still work.

Old command names (`checkin`, `add`, `remove`, `players`, `matches`, `mode`,
`pause`, `resume`, `roster`, `members`) still work but are not listed in `help`.

### Grades and the session recap

Grades set a new member's **starting rating**: A=1400, B=1200, C=1000,
D=800, E=600. After that, the rating goes up and down with results.

- Re-adding someone with `member` updates their grade but **keeps their
  earned rating**.
- `grade Bob A` changes Bob's grade **and resets his rating** to 1400.

After `stop`, the admin group gets a recap:

```text
📊 Session recap — 16 Nov · 8 matches
Player     G W-L  Rating     Grd
Alice      5 4-1 1452 +52   A
Bob        5 3-2 1261 +61   B→A
Dan        4 1-3  958 -42   C
Zed        0 0-0  800       D
```

`B→A` means Bob's rating now fits grade A. If you agree, `grade Bob A`.

### Troubleshooting

- **No reply to a command.** Make sure you're typing in the admin group, and
  the command is the whole message. Send `setup status` from the bot's phone
  in that group: it should say "This group: admin group". The backend log
  line `whatsapp_command_ignored` shows the reason
  (`announcement_group`, `unconfigured_group`, `too_old`).
- **"`setup` must be typed on the bot's own phone".** `setup` only works
  from the bot's number, so nobody else can take the bot over.
- **Lineups don't appear in the players' group.** Run `setup announce` there
  from the bot's phone. If only admins can send messages in that group, make
  the bot a group admin.
- **Commands sent while the backend was down** are skipped after two
  minutes; send them again.
- **The bot was unlinked** (removed under Linked devices on the bot phone):
  repeat the pairing in step 2. The group setup is kept.

### Privacy

The players' group sees player names, courts and scores. The bot stores no
phone numbers of players and never messages anyone privately.

### Upgrading from the `.env` setup

Older installs configured the bot with `WHATSAPP_CLUB_ID`,
`WHATSAPP_GROUP_JID` and `WHATSAPP_ADMIN_JIDS`. On the first start after
upgrading, the club and group are copied into the database (that group
becomes the players' group), and the listed admins can keep typing commands
there. To switch to the two-group setup, create the admin group and send
`setup admin <club name>` from the bot's phone there; from then on the
players' group only gets announcements, and the three variables can be
removed from `.env`.
