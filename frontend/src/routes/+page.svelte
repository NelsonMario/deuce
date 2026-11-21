<script lang="ts">
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

# WhatsApp bot
WHATSAPP_ENABLED=true

# Optional: AI natural language (Gemini)
GEMINI_API_KEY=your-api-key-here
GEMINI_MODEL=gemini-3.8-flash
GEMINI_PREFIX=chat
GEMINI_MAX_RPM=10`;

	const startSnippet = `cp .env.example .env
docker compose run --rm migrate
docker compose up -d --build backend`;

	const tokenSnippet = `curl -s -X POST http://localhost:8080/api/v1/clubs \\
  -H 'Content-Type: application/json' \\
  -d '{"club_name":"Tiger Club","host_display_name":"Host","host_gender":"MALE"}'

# copy "token" from the "host" object in the response
TOKEN='<host token>'`;

	const pairQrSnippet = `CODE=$(curl -s -H "Authorization: Bearer $TOKEN" \\
  http://localhost:8080/api/v1/whatsapp/qr \\
  | python3 -c 'import json,sys; print(json.load(sys.stdin).get("qr",""))')
npx -y qrcode@1.5.4 -o /tmp/whatsapp-qr.png "$CODE" && open /tmp/whatsapp-qr.png`;

	const rosterSnippet = `member
Alice F A
Bob M B
Cara F B
Mary Jane F C`;

	const sessionSnippet = `start
courts 2
in Alice, Bob, Cara, Dan, Eve
done 1 21-15      # Team A score first
set Eve Alice     # bench↔court swap
stop`;

	const aiSnippet = `chat start session with 2 courts
chat check in Alice, Bob, and Dan
chat Alice and Bob won 21-17 on court 1
chat who is waiting right now?`;

	const recapSnippet = `📊 Session recap — 16 Nov · 8 matches
Player     G W-L  Rating     Grd
Alice      5 4-1 1452 +52   A
Bob        5 3-2 1261 +61   B→A
Dan        4 1-3  958 -42   C`;

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

	const commandGroups = [
		{ title: 'Session', items: [['start / stop', 'Start or end session (stop posts recap)'], ['status', 'Courts, waiting list, recent results'], ['courts <n>', 'Set number of courts']] },
		{ title: 'Players', items: [['in <names>', 'Check members in; "in Eko M C" adds new member'], ['out <name>', 'Remove player from rotation'], ['member [name]', 'List members or show one'], ['member + lines', 'Bulk add/update "Name M/F Grade"'], ['grade <name> <A-E>', 'Change grade and reset rating']] },
		{ title: 'Matches', items: [['next', 'Fill a free court automatically'], ['done <court> <a>-<b>', 'Record result (Team A score first)'], ['cancel [court]', 'Undo latest or specific court match'], ['set <n1> <n2>', 'Swap players (bench↔court or court↔court)']] },
		{ title: 'Host & AI', items: [['auto | manual', 'Auto-fill courts or pick lineups yourself'], ['suggest <4 names>', 'Show most balanced split'], ['assign <court> A B vs C D', 'Set exact lineup'], ['chat <message>', 'Natural language command (optional Gemini)'], ['help', 'Show command list']] },
		{ title: 'Setup (bot phone)', items: [['setup admin <club>', 'Make this group the admin group'], ['setup announce', 'Make this group the players/announcement group'], ['setup status', 'Show current setup']] }
	];

	const toc = [
		['how-it-works', 'How it works'],
		['quick-start', 'Quick start'],
		['configuration', 'Configuration'],
		['session', 'Running a session'],
		['commands', 'Commands'],
		['grades', 'Grades & recap'],
		['troubleshooting', 'Troubleshooting'],
		['reset', 'Reset']
	];
</script>

<svelte:head>
	<title>Deuce — WhatsApp badminton session bot</title>
	<meta name="description" content="Simple docs for running Deuce: fair rotation, balanced doubles, results and recap in WhatsApp." />
</svelte:head>

<div class="page container-wide">
	<section class="hero">
		<p class="eyebrow">WhatsApp bot for badminton clubs</p>
		<h1>Run sessions, not spreadsheets.</h1>
		<p class="lead">
			Deuce rotates players fairly, builds balanced doubles, tracks results and posts a recap —
			all from WhatsApp. Hosts command in an admin group; players just see lineups and scores.
		</p>
		<div class="hero-actions">
			<a class="btn btn-primary" href="#quick-start">Quick start</a>
			<a class="btn btn-ghost" href="#commands">Command reference</a>
		</div>
		<div class="feature-grid">
			<div class="card feature"><strong>Fair rotation</strong><span>Queues players so nobody sits out unfairly long.</span></div>
			<div class="card feature"><strong>Balanced matches</strong><span>Uses grades/ratings and waiting time for doubles.</span></div>
			<div class="card feature"><strong>Two-group flow</strong><span>Admins control; players group gets announcements only.</span></div>
			<div class="card feature"><strong>Optional AI</strong><span>Prefix with <code>chat</code> for natural commands via Gemini.</span></div>
		</div>
	</section>

	<div class="layout">
		<aside class="sidebar">
			<nav class="toc card" aria-label="On this page">
				<strong>On this page</strong>
				<ol>
					{#each toc as [id, label]}<li><a href="#{id}">{label}</a></li>{/each}
				</ol>
			</nav>
		</aside>

		<article class="docs">
			<section id="how-it-works">
				<h2>How it works</h2>
				<p>Deuce uses two WhatsApp groups and never sends private messages.</p>
				<div class="flow">
					<div class="card flow-card">
						<span class="badge">Control</span>
						<h3>Admin group</h3>
						<p class="muted">You + co-hosts + bot</p>
						<ul><li>Type commands and get replies</li><li>Everyone in this group is an admin</li><li>Run sessions, manage members, get recap</li></ul>
						<pre class="mini"><code>in Alice Bob Cara Dan
done 1 21-15</code></pre>
					</div>
					<div class="flow-arrow">→</div>
					<div class="card flow-card">
						<span class="badge">Announce</span>
						<h3>Players' group</h3>
						<p class="muted">Players + bot</p>
						<ul><li>Session start/end</li><li>Lineups and results posted</li><li>Bot ignores chat here (no accidental triggers)</li></ul>
						<pre class="mini"><code>🏸 Court 1: Alice/Bob vs Cara/Dan
✅ Court 1: 21–15</code></pre>
					</div>
				</div>
			</section>

			<section id="quick-start">
				<h2>Quick start</h2>
				<p class="muted">You need Docker, a Postgres DB (e.g. Supabase), Node.js (for QR), a spare WhatsApp number for the bot, and the two groups.</p>
				<div class="steps">
					<div class="card step"><div class="step-n">1</div><div><h3>Configure & start backend</h3><p>Set DB and secret in <code>.env</code>, then run migrations and backend.</p><pre><code>{startSnippet}</code></pre></div></div>
					<div class="card step"><div class="step-n">2</div><div><h3>Create club & get token</h3><p>Token is needed for pairing the bot device.</p><pre><code>{tokenSnippet}</code></pre></div></div>
					<div class="card step"><div class="step-n">3</div><div><h3>Pair the bot (once)</h3><p>Use the <a href="/setup">Bot setup page</a> (recommended): paste host token, fetch QR, scan on bot phone: <strong>Settings → Linked devices → Link a device</strong>. Pairing survives restarts.</p><p class="muted">Backup (headless/scripted):</p><pre><code>{pairQrSnippet}</code></pre></div></div>
					<div class="card step"><div class="step-n">4</div><div><h3>Set up groups (on bot phone)</h3><p>Send from the bot's own number so nobody else can redirect it.</p>
						<table><thead><tr><th>In group</th><th>Send</th></tr></thead><tbody>
							<tr><td>Admin group</td><td><code>setup admin Tiger Club</code></td></tr>
							<tr><td>Players' group</td><td><code>setup announce</code></td></tr>
						</tbody></table></div></div>
					<div class="card step"><div class="step-n">5</div><div><h3>Add members</h3><p>In admin group, one per line: name, gender (M/F), grade (A–E).</p><pre><code>{rosterSnippet}</code></pre></div></div>
				</div>
			</section>

			<section id="configuration">
				<h2>Configuration</h2>
				<p>Infrastructure lives in <code>.env</code>. Club/groups/admins are configured from WhatsApp and stored in DB (no restart needed).</p>
				<pre><code>{envSnippet}</code></pre>
				<div class="note card">Do not put phone numbers, group IDs, or club IDs in <code>.env</code>. Old <code>WHATSAPP_CLUB_ID</code>, <code>WHATSAPP_GROUP_JID</code>, <code>WHATSAPP_ADMIN_JIDS</code> are obsolete.</div>
			</section>

			<section id="session">
				<h2>Running a session</h2>
				<p>All commands go in the <strong>admin group</strong>.</p>
				<pre><code>{sessionSnippet}</code></pre>
				<div class="cols">
					<div class="card"><h3>Auto vs manual</h3><p class="muted">Default auto-fills courts by rating + waiting time. Switch with <code>manual</code>/<code>auto</code>.</p><ul><li><code>suggest Alice Bob Cara Dan</code></li><li><code>assign 1 Alice Bob vs Cara Dan</code></li><li><code>next</code> fills one free court</li></ul></div>
					<div class="card"><h3>AI chat (optional)</h3><p class="muted">If <code>GEMINI_API_KEY</code> is set, prefix with <code>chat</code>.</p><pre class="mini"><code>{aiSnippet}</code></pre></div>
				</div>
				<p class="muted">Tip: names with spaces use commas, e.g. <code>in Mary Jane, Bob</code>.</p>
			</section>

			<section id="commands">
				<h2>Commands</h2>
				<div class="cmd-grid">
					{#each commandGroups as g}
						<div class="card cmd-card">
							<h3>{g.title}</h3>
							{#each g.items as [cmd, desc]}
								<div class="cmd-row"><code>{cmd}</code><span>{desc}</span></div>
							{/each}
						</div>
					{/each}
				</div>
			</section>

			<section id="grades">
				<h2>Grades & recap</h2>
				<p>Grades set starting rating; results move ratings over time. <code>grade Bob A</code> resets rating to that grade; re-importing via <code>member</code> keeps earned rating.</p>
				<table class="grades"><thead><tr><th>Grade</th><th>A</th><th>B</th><th>C</th><th>D</th><th>E</th></tr></thead><tbody><tr><td>Starting rating</td><td>1400</td><td>1200</td><td>1000</td><td>800</td><td>600</td></tr></tbody></table>
				<p>After <code>stop</code>, admin group gets a recap (e.g. <code>B→A</code> suggests re-grade):</p>
				<pre><code>{recapSnippet}</code></pre>
			</section>

			<section id="troubleshooting">
				<h2>Troubleshooting</h2>
				<pre><code>{logsSnippet}</code></pre>
				<table>
					<thead><tr><th>Log / issue</th><th>Meaning / fix</th></tr></thead>
					<tbody>
						<tr><td><code>whatsapp_command_received</code> → <code>whatsapp_reply_sent</code></td><td>Working.</td></tr>
						<tr><td><code>reason=announcement_group</code></td><td>Typed in players' group; use admin group.</td></tr>
						<tr><td><code>reason=unconfigured_group</code></td><td>Run <code>setup admin</code> there from bot phone.</td></tr>
						<tr><td><code>reason=too_old</code></td><td>Sent while backend down; resend.</td></tr>
						<tr><td>"setup must be typed on the bot's own phone"</td><td><code>setup</code> only works from bot number.</td></tr>
						<tr><td>No lineups in players' group</td><td>Run <code>setup announce</code>; make bot group admin if posting restricted.</td></tr>
						<tr><td>Bot unlinked</td><td>Re-pair (step 3). Group setup is kept.</td></tr>
					</tbody>
				</table>
				<p class="muted"><code>GET /api/v1/whatsapp/status</code> (with token) shows connection and configured groups. Use the <a href="/setup">Bot setup page</a> for a UI, or curl as backup.</p>
			</section>

			<section id="reset">
				<h2>Resetting the database</h2>
				<p>For a clean start, drop tables and enum types (dropping tables only leaves enums and breaks next migration). Bot pairing (<code>whatsmeow</code> schema) is kept.</p>
				<pre><code>{resetSnippet}</code></pre>
				<p class="muted">Then re-run start/migrate, create token, and redo group/member setup.</p>
			</section>
		</article>
	</div>
</div>

<style>
	.page { padding-top: 8px; }
	.hero { padding: 28px 0 36px; border-bottom: 1px solid var(--border-soft); margin-bottom: 28px; }
	.eyebrow { color: var(--accent); font-weight: 600; font-size: 0.9rem; margin-bottom: 10px; }
	h1 { font-size: clamp(2rem, 4vw, 3.1rem); line-height: 1.05; letter-spacing: -0.04em; margin: 0 0 14px; max-width: 14ch; }
	.lead { color: var(--text-dim); font-size: 1.1rem; max-width: 62ch; margin: 0 0 22px; line-height: 1.6; }
	.hero-actions { display: flex; gap: 10px; flex-wrap: wrap; margin-bottom: 26px; }
	.feature-grid { display: grid; grid-template-columns: repeat(4, 1fr); gap: 12px; }
	.feature { padding: 16px; display: flex; flex-direction: column; gap: 6px; }
	.feature strong { font-size: 0.98rem; }
	.feature span { color: var(--text-dim); font-size: 0.9rem; line-height: 1.45; }

	.layout { display: grid; grid-template-columns: 220px minmax(0, 1fr); gap: 28px; align-items: start; }
	.sidebar { position: sticky; top: 84px; }
	.toc { padding: 14px 16px; }
	.toc strong { font-size: 0.85rem; color: var(--text-dim); text-transform: uppercase; letter-spacing: .04em; }
	.toc ol { margin: 10px 0 0; padding-left: 18px; display: grid; gap: 6px; }
	.toc a { color: var(--text-dim); font-size: 0.92rem; }
	.toc a:hover { color: var(--text); }

	.docs section { scroll-margin-top: 88px; padding: 10px 0 34px; border-bottom: 1px solid var(--border-soft); }
	.docs section:last-child { border-bottom: 0; }
	h2 { font-size: 1.55rem; margin: 0 0 12px; letter-spacing: -0.03em; }
	h3 { font-size: 1rem; margin: 0 0 6px; }
	p, li { line-height: 1.65; }
	a { color: var(--accent); }
	.muted { color: var(--text-dim); }

	.flow { display: grid; grid-template-columns: 1fr auto 1fr; gap: 12px; align-items: stretch; margin: 14px 0; }
	.flow-card { padding: 18px; }
	.flow-card ul { margin: 10px 0; padding-left: 18px; color: var(--text-dim); }
	.flow-arrow { display:flex; align-items:center; justify-content:center; color: var(--text-faint); font-size: 1.4rem; }
	.badge { display:inline-block; font-size:.75rem; font-weight:700; color: var(--accent-contrast); background: var(--accent); padding:3px 8px; border-radius:999px; margin-bottom:8px; }

	.steps { display: grid; gap: 12px; margin-top: 14px; }
	.step { display: grid; grid-template-columns: auto 1fr; gap: 14px; padding: 16px 18px; align-items: start; }
	.step-n { width: 30px; height: 30px; border-radius: 999px; background: color-mix(in srgb, var(--accent) 18%, transparent); color: var(--accent); display:flex; align-items:center; justify-content:center; font-weight:700; }

	.cols, .cmd-grid { display: grid; grid-template-columns: repeat(2, minmax(0,1fr)); gap: 12px; margin: 14px 0; }
	.cmd-card { padding: 16px; }
	.cmd-row { display: grid; gap: 4px; padding: 10px 0; border-top: 1px solid var(--border-soft); }
	.cmd-row:first-of-type { border-top: 0; }
	.cmd-row span { color: var(--text-dim); font-size: 0.92rem; }

	pre { background: #0f1115; border: 1px solid var(--border-soft); border-radius: 14px; padding: 14px 16px; overflow-x: auto; line-height: 1.5; margin: 12px 0; }
	pre.mini { padding: 10px 12px; font-size: 0.85rem; }
	code { font-family: var(--font-mono); font-size: 0.88em; }
	:not(pre) > code { background: var(--bg-elevated-2); border: 1px solid var(--border-soft); border-radius: 6px; padding: 1px 6px; }

	table { width: 100%; border-collapse: collapse; margin: 12px 0; font-size: 0.93rem; display: block; overflow-x: auto; }
	th, td { text-align: left; vertical-align: top; padding: 10px 12px; border-bottom: 1px solid var(--border-soft); }
	th { color: var(--text-dim); font-weight: 600; font-size: 0.85rem; }
	.note { margin-top: 12px; padding: 14px 16px; color: var(--text-dim); border-left: 3px solid var(--accent); }
	.grades td, .grades th { text-align: center; }

	@media (max-width: 960px) {
		.feature-grid { grid-template-columns: repeat(2, 1fr); }
		.layout { grid-template-columns: 1fr; }
		.sidebar { position: static; }
		.toc ol { grid-template-columns: repeat(2, minmax(0,1fr)); display:grid; }
		.flow { grid-template-columns: 1fr; }
		.flow-arrow { transform: rotate(90deg); }
	}
	@media (max-width: 640px) {
		.feature-grid, .cols, .cmd-grid { grid-template-columns: 1fr; }
		h1 { max-width: none; }
		.toc ol { grid-template-columns: 1fr; }
	}
</style>
