<script lang="ts">
	// Code samples live here so braces in them don't clash with Svelte syntax.
	const envSnippet = `# Database (Supabase: use the session pooler details and require SSL)
DB_HOST=aws-0-ap-northeast-1.pooler.supabase.com
DB_PORT=5432
DB_USER=postgres.<project-ref>
DB_PASSWORD=<database password>
DB_NAME=postgres
DB_SSLMODE=require

# Secret for hashing API tokens: openssl rand -hex 32
PLAYER_TOKEN_SECRET=<random secret>

# Optional Redis (rate limiting, locks). Leave REDIS_HOST unset to skip it.
REDIS_HOST=
REDIS_PORT=6379
REDIS_USERNAME=default
REDIS_PASSWORD=
REDIS_TLS=false

# WhatsApp bot. Everything else about the bot is set up from WhatsApp.
WHATSAPP_ENABLED=true`;

	const startSnippet = `docker compose run --rm migrate          # applies migrations 1-6
docker compose up -d --build backend`;

	const tokenSnippet = `curl -s -X POST http://localhost:8080/api/v1/clubs \\
  -H 'Content-Type: application/json' \\
  -d '{"club_name":"Tiger Club","host_display_name":"Host","host_gender":"MALE"}'

# copy "token" from the "host" object in the response
TOKEN='<host token>'`;

	const pairCheckSnippet = `curl -s -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/v1/whatsapp/qr`;

	const pairQrSnippet = `CODE=$(curl -s -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/v1/whatsapp/qr \\
  | python3 -c 'import json,sys; print(json.load(sys.stdin).get("qr",""))')
npx -y qrcode@1.5.4 -o /tmp/whatsapp-qr.png "$CODE" && open /tmp/whatsapp-qr.png`;

	const rosterSnippet = `member
Alice F A
Bob M B
Cara F B
Mary Jane F C`;

	const sessionSnippet = `start
courts 2
in Alice Bob Cara Dan Eve
done 1 21-15
stop`;

	const recapSnippet = `📊 Session recap — 16 Nov · 8 matches
Player     G W-L  Rating     Grd
Alice      5 4-1 1452 +52   A
Bob        5 3-2 1261 +61   B→A
Dan        4 1-3  958 -42   C
Zed        0 0-0  800       D`;

	const logsSnippet = `docker compose logs -f backend | grep whatsapp`;

	const resetSnippet = `DROP TABLE IF EXISTS
  whatsapp_bot_config, whatsapp_group_outbox, rating_history, match_players, matches,
  courts, session_players, sessions, player_ratings, club_members, clubs,
  player_tokens, players, schema_migrations
CASCADE;
DROP TYPE IF EXISTS
  club_member_role, player_gender, session_status, session_assignment_mode,
  session_player_status, court_status, match_format, match_status, match_team, match_winner
CASCADE;`;

	const commands: { area: string; cmd: string; desc: string }[] = [
		{ area: "Session", cmd: "start", desc: "Start a session" },
		{ area: "", cmd: "stop", desc: "End the session and post the recap" },
		{ area: "", cmd: "status", desc: "Courts with lineups, who's waiting, latest results" },
		{ area: "Players", cmd: "in <names>", desc: "Check members in; \"in Eko M C\" also adds a new member" },
		{ area: "", cmd: "out <name>", desc: "Take a player out of the rotation" },
		{ area: "Courts", cmd: "courts <n>", desc: "Set the number of courts (\"courts\" alone lists them)" },
		{ area: "", cmd: "next", desc: "Fill the next free court automatically" },
		{ area: "", cmd: "done <court> <a>-<b>", desc: "Record a result (Team A first) and free the court" },
		{ area: "", cmd: "cancel", desc: "Cancel the latest unfinished match" },
		{ area: "Host", cmd: "auto | manual", desc: "Bot fills courts / you pick lineups" },
		{ area: "", cmd: "suggest <4 names>", desc: "Show the most balanced split" },
		{ area: "", cmd: "assign <court> A B vs C D", desc: "Put an exact lineup on a court" },
		{ area: "Member", cmd: "member [name]", desc: "List members, or show one" },
		{ area: "", cmd: "member + lines", desc: "Bulk add/update, one \"Name M/F Grade\" per line" },
		{ area: "", cmd: "grade <name> <A-E>", desc: "Change a grade and reset the rating to it" },
		{ area: "Setup", cmd: "setup admin <club>", desc: "Bot phone only: make this group the admin group" },
		{ area: "", cmd: "setup announce", desc: "Bot phone only: make this group the players' group" },
		{ area: "", cmd: "setup status", desc: "Bot phone only: show the current setup" },
		{ area: "", cmd: "help", desc: "Command list" },
	];

	const toc = [
		["overview", "Overview"],
		["requirements", "Requirements"],
		["configuration", "Configuration"],
		["setup", "Setting up the bot"],
		["admins", "Admins and co-hosts"],
		["session", "Running a session"],
		["commands", "Commands"],
		["grades", "Grades and recap"],
		["troubleshooting", "Troubleshooting"],
		["reset", "Resetting the database"],
	];
</script>

<svelte:head>
	<title>Deuce Documentation</title>
	<meta name="description" content="Set up and run the Deuce WhatsApp bot for badminton sessions." />
</svelte:head>

<article class="container-wide docs">
	<h1>Deuce</h1>
	<p class="lead">
		A WhatsApp bot that runs badminton sessions: fair court rotation, rating-balanced doubles,
		match results and a session recap. Hosts control it from an admin group; players follow
		along in the club group.
	</p>

	<nav class="toc" aria-label="Contents">
		<strong>Contents</strong>
		<ol>
			{#each toc as [id, label]}
				<li><a href="#{id}">{label}</a></li>
			{/each}
		</ol>
	</nav>

	<section id="overview">
		<h2>Overview</h2>
		<p>The bot works with two WhatsApp groups:</p>
		<table>
			<thead>
				<tr><th>Group</th><th>Members</th><th>What happens there</th></tr>
			</thead>
			<tbody>
				<tr>
					<td>Admin group</td>
					<td>You, co-hosts, the bot</td>
					<td>Hosts type commands; the bot replies here. Everyone in this group is an admin.</td>
				</tr>
				<tr>
					<td>Players' group</td>
					<td>Players, the bot</td>
					<td>Only announcements: session start/end, lineups and results. The bot ignores messages here.</td>
				</tr>
			</tbody>
		</table>
		<p>
			The bot never sends private messages and ignores private chats. Its setup is stored in the
			database and changed from WhatsApp, so no configuration file edits or restarts are needed
			after the first install.
		</p>
	</section>

	<section id="requirements">
		<h2>Requirements</h2>
		<ul>
			<li>Docker, and Node.js (only to draw the pairing QR code).</li>
			<li>A PostgreSQL database (e.g. Supabase).</li>
			<li>
				A separate WhatsApp number for the bot, on a spare phone. Do not use your personal number:
				the bot posts as that account, and messages typed on the bot phone count as setup commands.
			</li>
			<li>The two WhatsApp groups above, with the bot number added to both.</li>
		</ul>
	</section>

	<section id="configuration">
		<h2>Configuration</h2>
		<p>
			Copy <code>.env.example</code> to <code>.env</code> in the repository root and fill in the
			database values:
		</p>
		<pre><code>{envSnippet}</code></pre>
		<table>
			<thead>
				<tr><th>Variable</th><th>Purpose</th></tr>
			</thead>
			<tbody>
				<tr><td><code>DB_*</code></td><td>Database connection. Use <code>DB_SSLMODE=require</code> for Supabase.</td></tr>
				<tr><td><code>PLAYER_TOKEN_SECRET</code></td><td>Secret used to hash API tokens.</td></tr>
				<tr><td><code>REDIS_*</code></td><td>Optional. Shared rate limiting and locks; the backend runs without it.</td></tr>
				<tr><td><code>WHATSAPP_ENABLED</code></td><td>Starts the bot (default <code>true</code>). Set <code>false</code> to run the API only.</td></tr>
			</tbody>
		</table>
		<p>
			No phone numbers, group IDs or club IDs go in <code>.env</code>. The older
			<code>WHATSAPP_CLUB_ID</code>, <code>WHATSAPP_GROUP_JID</code> and
			<code>WHATSAPP_ADMIN_JIDS</code> variables are no longer needed and can be removed.
		</p>
	</section>

	<section id="setup">
		<h2>Setting up the bot</h2>

		<h3>1. Start the backend</h3>
		<pre><code>{startSnippet}</code></pre>

		<h3>2. Get an API token</h3>
		<p>Pairing needs an API token. Creating the club returns one:</p>
		<pre><code>{tokenSnippet}</code></pre>

		<h3>3. Pair the bot (once)</h3>
		<pre><code>{pairCheckSnippet}</code></pre>
		<p>
			If it returns <code>{`{"status":"PAIRED"}`}</code>, skip to step 4. Otherwise generate the QR
			code and open it:
		</p>
		<pre><code>{pairQrSnippet}</code></pre>
		<p>
			On the bot phone, open <strong>WhatsApp → Settings → Linked devices → Link a device</strong>
			and scan it. Codes expire after about 20 seconds; if the scan fails or the first call said
			<code>WAITING</code>, run it again. The pairing is stored in the database and survives
			restarts.
		</p>

		<h3>4. Set up the groups</h3>
		<p>Type these <strong>on the bot phone</strong>:</p>
		<table>
			<thead>
				<tr><th>In this group</th><th>Type</th><th>Result</th></tr>
			</thead>
			<tbody>
				<tr><td>Admin group</td><td><code>setup admin Tiger Club</code></td><td>Becomes the admin group for the club</td></tr>
				<tr><td>Players' group</td><td><code>setup announce</code></td><td>Becomes the announcement group</td></tr>
			</tbody>
		</table>
		<p>
			Use the same club name as in step 2 to reuse that club. <code>setup status</code> shows the
			current setup; running a <code>setup</code> command in another group moves that role there.
			Only the bot phone can run <code>setup</code>, so nobody else can redirect the bot.
		</p>

		<h3>5. Add your members</h3>
		<p>In the admin group, one member per line: name, gender (M/F), grade (A strongest to E).</p>
		<pre><code>{rosterSnippet}</code></pre>
		<p>Check the result with <code>member</code>.</p>
	</section>

	<section id="admins">
		<h2>Admins and co-hosts</h2>
		<p>
			Admin access is membership of the admin group. The bot does not store admin phone numbers.
		</p>
		<ul>
			<li><strong>Add a co-host:</strong> add them to the admin group in WhatsApp.</li>
			<li><strong>Remove a co-host:</strong> remove them from the admin group. It takes effect immediately.</li>
		</ul>
		<p>
			Because the group is the access control, lock it down. In the admin group's info, open
			<strong>Group settings</strong> and set:
		</p>
		<ul>
			<li><strong>Add other members → Only admins</strong>, so only you (the WhatsApp group admin) decide who becomes a co-host.</li>
			<li><strong>Edit group settings → Only admins.</strong></li>
		</ul>
		<p>
			Optionally set the players' group to <strong>Send messages → Only admins</strong> and make the
			bot a group admin there, so it only contains announcements. The bot must be a group admin to
			post in a group with that setting.
		</p>
	</section>

	<section id="session">
		<h2>Running a session</h2>
		<p>All commands go in the admin group. A command without arguments must be the whole message.</p>
		<pre><code>{sessionSnippet}</code></pre>
		<ol>
			<li><code>start</code> starts the session; the players' group is notified.</li>
			<li><code>courts 2</code> sets the number of courts.</li>
			<li>
				<code>in …</code> checks members in. For someone new, add gender and grade:
				<code>in Zed M C</code>.
			</li>
			<li>
				Free courts are filled automatically by rating and waiting time; each lineup is posted to
				the players' group.
			</li>
			<li>
				After each game, <code>done 1 21-15</code> records the result (court, then Team A's score),
				updates ratings, posts the result and frees the court.
			</li>
			<li><code>stop</code> ends the session and posts the recap in the admin group.</li>
		</ol>
		<p>
			To pick matches yourself: <code>manual</code>, then <code>suggest Alice Bob Cara Dan</code>
			for a balanced split and <code>assign 1 Alice Bob vs Cara Dan</code> to place it.
			<code>auto</code> switches back.
		</p>
	</section>

	<section id="commands">
		<h2>Commands</h2>
		<table>
			<thead>
				<tr><th>Area</th><th>Command</th><th>Description</th></tr>
			</thead>
			<tbody>
				{#each commands as c}
					<tr>
						<td>{c.area}</td>
						<td><code>{c.cmd}</code></td>
						<td>{c.desc}</td>
					</tr>
				{/each}
			</tbody>
		</table>
		<p>Names with spaces are separated by commas: <code>in Mary Jane, Bob</code>.</p>
	</section>

	<section id="grades">
		<h2>Grades and recap</h2>
		<p>Grades set a new member's starting rating; after that the rating moves with results.</p>
		<table>
			<thead>
				<tr><th>Grade</th><th>A</th><th>B</th><th>C</th><th>D</th><th>E</th></tr>
			</thead>
			<tbody>
				<tr><td>Starting rating</td><td>1400</td><td>1200</td><td>1000</td><td>800</td><td>600</td></tr>
			</tbody>
		</table>
		<ul>
			<li>Re-importing a member with <code>roster</code> updates the grade but keeps the earned rating.</li>
			<li><code>grade Bob A</code> changes the grade and resets the rating to 1400.</li>
		</ul>
		<p>After <code>stop</code>, the admin group receives a recap:</p>
		<pre><code>{recapSnippet}</code></pre>
		<p><code>B→A</code> means the player's rating now fits grade A.</p>
	</section>

	<section id="troubleshooting">
		<h2>Troubleshooting</h2>
		<pre><code>{logsSnippet}</code></pre>
		<table>
			<thead>
				<tr><th>Log line</th><th>Meaning / fix</th></tr>
			</thead>
			<tbody>
				<tr><td><code>whatsapp_command_received</code> then <code>whatsapp_reply_sent</code></td><td>Working.</td></tr>
				<tr><td><code>whatsapp_command_ignored reason=announcement_group</code></td><td>Typed in the players' group; use the admin group.</td></tr>
				<tr><td><code>whatsapp_command_ignored reason=unconfigured_group</code></td><td>That group is not set up; run <code>setup admin</code> there from the bot phone.</td></tr>
				<tr><td><code>whatsapp_command_ignored reason=too_old</code></td><td>Sent while the backend was down; send it again.</td></tr>
			</tbody>
		</table>
		<ul>
			<li><strong>"setup must be typed on the bot's own phone":</strong> <code>setup</code> only works from the bot number.</li>
			<li><strong>No lineups in the players' group:</strong> run <code>setup announce</code> there; if only admins can post, make the bot a group admin.</li>
			<li><strong>Bot unlinked:</strong> repeat step 3. The group setup is kept.</li>
		</ul>
		<p>
			<code>GET /api/v1/whatsapp/status</code> (with the token) shows the connection and which groups
			are set up.
		</p>
	</section>

	<section id="reset">
		<h2>Resetting the database</h2>
		<p>
			For a clean start or an end-to-end test, run this in the database's SQL editor. Deleting
			tables one by one leaves the enum types behind, and the next migration then fails with
			<code>type "club_member_role" already exists</code>.
		</p>
		<pre><code>{resetSnippet}</code></pre>
		<p>
			Then repeat steps 1, 2, 4 and 5. The bot stays paired, because its <code>whatsmeow</code>
			schema is kept.
		</p>
	</section>
</article>

<style>
	.docs {
		max-width: 820px;
		line-height: 1.6;
	}

	h1 {
		font-size: 2rem;
		margin: 0 0 8px;
	}

	.lead {
		color: var(--text-dim);
		font-size: 1.05rem;
		margin: 0 0 24px;
	}

	h2 {
		font-size: 1.4rem;
		margin: 40px 0 12px;
		padding-bottom: 6px;
		border-bottom: 1px solid var(--border);
		scroll-margin-top: 72px;
	}

	h3 {
		font-size: 1.05rem;
		margin: 24px 0 8px;
	}

	section {
		scroll-margin-top: 72px;
	}

	a {
		color: var(--accent);
	}

	.toc {
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
		padding: 12px 16px;
		background: var(--bg-elevated);
	}

	.toc ol {
		margin: 8px 0 0;
		padding-left: 20px;
		columns: 2;
		column-gap: 32px;
	}

	@media (max-width: 560px) {
		.toc ol {
			columns: 1;
		}
	}

	code {
		font-family: var(--font-mono);
		font-size: 0.88em;
		background: var(--bg-elevated-2);
		border-radius: 4px;
		padding: 1px 5px;
	}

	pre {
		background: var(--bg-elevated);
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
		padding: 12px 14px;
		overflow-x: auto;
		line-height: 1.45;
	}

	pre code {
		background: none;
		padding: 0;
		font-size: 0.85rem;
	}

	table {
		width: 100%;
		border-collapse: collapse;
		margin: 12px 0;
		font-size: 0.92rem;
		display: block;
		overflow-x: auto;
	}

	th,
	td {
		text-align: left;
		vertical-align: top;
		padding: 8px 10px;
		border-bottom: 1px solid var(--border-soft);
	}

	th {
		color: var(--text-dim);
		font-weight: 600;
		border-bottom: 1px solid var(--border);
	}

	td code {
		white-space: nowrap;
	}

	ul,
	ol {
		padding-left: 22px;
	}

	li {
		margin: 4px 0;
	}
</style>
