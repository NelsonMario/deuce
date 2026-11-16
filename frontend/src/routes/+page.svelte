<script lang="ts">
	import Typewriter from "$lib/components/Typewriter.svelte";
<<<<<<< Updated upstream
	import { identity } from "$lib/stores/identity";
=======
	import Marquee from "$lib/components/Marquee.svelte";
	import SpinBadge from "$lib/components/SpinBadge.svelte";
	import { reveal, tilt, magnetic, trackPointer } from "$lib/actions/motion";
>>>>>>> Stashed changes

	const phrases = [
		"Court 2 is ready: Alice / Bob vs Cara / Dan. 🏸",
		"Host manages rotation right from WhatsApp.",
		"Zero court politics. Balanced matches every time.",
		"No more asking “who's playing next?”",
	];

<<<<<<< Updated upstream
	let lastSessionId = $derived($identity.lastSessionId);
=======
	const ticker = [
		"Host types `next`",
		"Group sees the match assignment",
		"Head straight to court",
		"Ratings update automatically",
		"Zero app clutter",
	];

	// Interactive Chat Simulator Tab
	let simTab = $state<"host" | "player">("player");
>>>>>>> Stashed changes
</script>

<svelte:head>
	<title>Deuce — WhatsApp Badminton Court Manager</title>
</svelte:head>

<<<<<<< Updated upstream
<section class="container hero halftone rise-in">
	<p class="kicker">Smart(?) Court Rotation</p>
	<h1>
		<Typewriter {phrases} />
	</h1>
	<p class="sub">
		Build your club. Bring your crew. We’ll handle the rotation.
	</p>

	<div class="cta stack">
		<a href="/host" class="btn btn-primary btn-block">Host a session</a>
		<a href="/join" class="btn btn-ghost btn-block">Join with a code</a>
	</div>

	{#if lastSessionId}
		<a href="/session/{lastSessionId}" class="continue">
			↳ Continue where you left off
		</a>
	{/if}
</section>

<section class="container-wide how">
	<div class="how-grid">
		<div class="how-card card card-tight rise-in">
			<span class="how-num">01</span>
			<h3>Create</h3>
			<p class="muted">
				Host makes a club and a session, gets a join code to share on
				the spot.
			</p>
		</div>
		<div class="how-card card card-tight rise-in">
			<span class="how-num">02</span>
			<h3>Join</h3>
			<p class="muted">
				Players scan or type the code, add their name, and land in the
				queue.
			</p>
		</div>
		<div class="how-card card card-tight rise-in">
			<span class="how-num">03</span>
			<h3>Play</h3>
			<p class="muted">
				Matches are generated from ratings and wait time. Scores feed
				the next one.
			</p>
=======
<!-- HERO SECTION -->
<section class="hero-band halftone rise-in" use:trackPointer>
	<div class="hero-fx" aria-hidden="true">
		<span class="watermark">WHATSAPP</span>
		<div class="spotlight"></div>
	</div>

	<div class="container hero">
		<div class="spin-slot" use:reveal={{ delay: 100 }}>
			<SpinBadge />
		</div>

		<!-- Status Pill -->
		<div class="status-pill-wrap" use:reveal={{ delay: 160 }}>
			<span class="status-pill online">
				<span class="dot"></span>
				One Group. One Source of Truth.
			</span>
		</div>

		<h1 use:reveal={{ delay: 240 }}>
			<Typewriter {phrases} />
		</h1>

		<p class="sub" use:reveal={{ delay: 320 }}>
			Host your badminton session without constantly checking the web app.
			Control rotations in one WhatsApp group, where everyone sees the same court assignments.
		</p>

		<div class="cta-row" use:reveal={{ delay: 400 }}>
			<a href="#group-section" class="btn btn-primary magnetic" use:magnetic>
				<span>See Group Workflow</span>
				<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2.6" stroke-linecap="round" stroke-linejoin="round">
					<path d="M5 12h14M13 6l6 6-6 6" />
				</svg>
			</a>
			<a href="#host-section" class="btn btn-ghost">
				Host Setup
			</a>
		</div>
	</div>
</section>

<!-- TICKER -->
<div class="ticker-band rise-in">
	<Marquee items={ticker} speed={32} />
</div>

<!-- INTERACTIVE DEMO / SHOWCASE -->
<section class="container-wide showcase-section">
	<div class="section-header" use:reveal>
		<p class="section-kicker">Interactive Experience</p>
		<h2>How It Works On WhatsApp</h2>
		<p class="muted max-w">Zero app downloads. Everything runs through your existing WhatsApp chats.</p>
	</div>

	<div class="chat-showcase-grid">
		<!-- Phone Mockup -->
		<div class="phone-mockup" use:tilt={{ max: 5 }}>
			<div class="phone-header">
				<div class="avatar">🏸</div>
				<div class="phone-title">
					<h4>Deuce Badminton Bot</h4>
					<span class="status-text">online • badminton engine</span>
				</div>
			</div>

			<div class="chat-body">
				<div class="date-badge">TODAY</div>

				{#if simTab === 'player'}
					<!-- Player View -->
					<div class="chat-bubble user">
						<span class="bubble-text">status</span>
						<span class="bubble-time">19:04</span>
					</div>

					<div class="chat-bubble bot">
						<span class="bubble-text">
							🏸 <b>Session Status</b><br/><br/>
							• <b>Status</b>: ACTIVE<br/>
							• <b>Courts</b>: 3<br/>
							• <b>Waiting Queue</b>: 4 players<br/><br/>
							Everyone can see the same queue and court status.
						</span>
						<span class="bubble-time">19:04</span>
					</div>

					<div class="chat-bubble bot highlight">
						<span class="bubble-text">
							🏸 <b>Match Assignment</b><br/><br/>
							<b>Court</b>: Court 2<br/>
							<b>Format</b>: MIXED_DOUBLES<br/><br/>
							Alice / Bob<br/>
							vs<br/>
							Charlie / David
						</span>
						<span class="bubble-time">19:12</span>
					</div>
				{:else}
					<!-- Host View -->
					<div class="chat-bubble user">
						<span class="bubble-text">start</span>
						<span class="bubble-time">19:00</span>
					</div>

					<div class="chat-bubble bot">
						<span class="bubble-text">
							🏸 <b>Session Started!</b><br/>
							Session <i>Friday Smash</i> is now ACTIVE.<br/>
							Autofill: enabled
						</span>
						<span class="bubble-time">19:00</span>
					</div>

					<div class="chat-bubble user">
						<span class="bubble-text">next</span>
						<span class="bubble-time">19:02</span>
					</div>

					<div class="chat-bubble bot highlight">
						<span class="bubble-text">
							🏸 <b>Match Generated!</b><br/>
							Court: <b>Court 1</b><br/>
							Format: MIXED_DOUBLES<br/>
							✅ Assignment posted to the group.
						</span>
						<span class="bubble-time">19:02</span>
					</div>
				{/if}
			</div>

			<div class="chat-tabs">
				<button class="tab-btn {simTab === 'player' ? 'active' : ''}" onclick={() => (simTab = 'player')}>
					Public Group View
				</button>
				<button class="tab-btn {simTab === 'host' ? 'active' : ''}" onclick={() => (simTab = 'host')}>
					Host Controls
				</button>
			</div>
		</div>

		<!-- Feature Bullets -->
		<div class="feature-bullets">
			<div class="feature-card card" use:reveal={{ delay: 100 }}>
				<div class="icon-wrap">⚡</div>
				<div>
					<h3>Public Assignments</h3>
					<p class="muted">Every match assignment is posted once in the club group. Everyone sees the court and all four names.</p>
				</div>
			</div>

			<div class="feature-card card" use:reveal={{ delay: 200 }}>
				<div class="icon-wrap">🎮</div>
				<div>
					<h3>Host In-Chat Commands</h3>
					<p class="muted">Approved admins control the existing Deuce session with <code>next</code>, <code>pause</code>, and <code>resume</code>.</p>
				</div>
			</div>

			<div class="feature-card card" use:reveal={{ delay: 300 }}>
				<div class="icon-wrap">🔒</div>
				<div>
					<h3>Strict Host Authorization</h3>
					<p class="muted">Administrative commands are restricted to configured WhatsApp JIDs. Display names are never trusted.</p>
				</div>
			</div>

			<div class="feature-card card" use:reveal={{ delay: 400 }}>
				<div class="icon-wrap">🛡️</div>
				<div>
					<h3>Transactional Reliability</h3>
					<p class="muted">Backed by a resilient PostgreSQL outbox. Temporary WhatsApp disconnections won't drop match alerts; they retry safely with backoff.</p>
				</div>
			</div>
		</div>
	</div>
</section>

<!-- GROUP WORKFLOW -->
<section id="group-section" class="container-wide pairing-section">
	<div class="pairing-card card" use:reveal>
		<div class="pairing-content">
			<div class="badge-tag">PUBLIC GROUP</div>
			<h2>No Player Pairing Required</h2>
			<p class="muted">Add the bot to the club group. Players simply read <code>status</code>, <code>players</code>, and <code>matches</code>; no phone number is linked to a Deuce player.</p>
		</div>
	</div>
</section>

<!-- HOST SETUP & QR CODE SECTION -->
<section id="host-section" class="container-wide host-setup-section">
	<div class="host-grid">
		<div class="host-info" use:reveal>
			<div class="badge-tag host">HOST SETUP</div>
			<h2>WhatsApp Bot Pairing</h2>
			<p class="muted">
				An admin links the dedicated Deuce Bot account to the club group once using WhatsApp's companion-device flow.
			</p>

			<div class="setup-steps">
				<div class="step-item">
					<span class="step-num">1</span>
					<p>Open <b>WhatsApp</b> on your dedicated bot phone.</p>
				</div>
				<div class="step-item">
					<span class="step-num">2</span>
					<p>Go to <b>Settings > Linked Devices > Link a Device</b>.</p>
				</div>
				<div class="step-item">
					<span class="step-num">3</span>
					<p>Scan the QR code shown on this station.</p>
				</div>
			</div>

			<div class="callout-box">
				<p>💡 <b>Pro-Tip</b>: Use a dedicated prepaid SIM card for Deuce Bot to isolate club alerts from your personal WhatsApp account.</p>
			</div>
		</div>

		<div class="qr-station card" use:tilt={{ max: 4 }}>
			<h3>Device Pairing Station</h3>
			<div class="station-badge scan">
				📲 Admin setup
			</div>

			<div class="qr-canvas-wrap">
				<div class="qr-success">
					<span class="huge-icon">🏸</span>
					<h4>One shared group</h4>
					<p class="muted">The secure admin setup screen provides the QR code and connection status. The public site never exposes bot credentials.</p>
				</div>
			</div>

			<div class="station-meta">
				<span>Players need no setup—just join the group.</span>
			</div>
		</div>
	</div>
</section>

<!-- COMMAND REFERENCE -->
<section class="container-wide commands-section">
	<div class="section-header" use:reveal>
		<p class="section-kicker">Quick Reference</p>
		<h2>Commands Cheat Sheet</h2>
	</div>

	<div class="commands-grid">
		<div class="command-card card">
			<h3>Admin Commands</h3>
			<ul class="cmd-list">
				<li><code>status</code> <span>View session summary and court allocations</span></li>
				<li><code>start</code> <span>Start the badminton session</span></li>
				<li><code>stop</code> <span>Finish the session and stop all alerts</span></li>
				<li><code>next</code> <span>Spin up next match on available court</span></li>
				<li><code>pause</code> <span>Pause automatic court filling</span></li>
				<li><code>resume</code> <span>Resume automatic matchmaking</span></li>
				<li><code>players</code> <span>Show the current player queue</span></li>
				<li><code>add &lt;name&gt; [M/F]</code> <span>Add a player to the session</span></li>
				<li><code>courts &lt;n&gt;</code> <span>Add courts up to the requested count</span></li>
				<li><code>cancel</code> <span>Cancel the latest active match</span></li>
			</ul>
		</div>

		<div class="command-card card">
			<h3>Public Commands</h3>
			<ul class="cmd-list">
				<li><code>status</code> <span>View shared session and queue status</span></li>
				<li><code>players</code> <span>View the current player list</span></li>
				<li><code>matches</code> <span>View match states</span></li>
				<li><code>help</code> <span>Show commands available to you</span></li>
			</ul>
>>>>>>> Stashed changes
		</div>
	</div>
</section>

<style>
	.hero {
<<<<<<< Updated upstream
		padding-top: 8vh;
		text-align: center;
	}

	.kicker {
		text-transform: uppercase;
		letter-spacing: 0.14em;
		font-size: 0.72rem;
		font-weight: 700;
		color: var(--accent);
		margin-bottom: 14px;
	}

	h1 {
		font-size: clamp(1.9rem, 6vw, 2.6rem);
		font-weight: 800;
		letter-spacing: -0.02em;
		line-height: 1.15;
		min-height: 2.4em;
=======
		position: relative;
		padding-top: 8vh;
		padding-bottom: 8vh;
		text-align: center;
	}

	.hero-fx {
		position: absolute;
		inset: 0;
		z-index: -1;
		pointer-events: none;
	}

	.watermark {
		position: absolute;
		left: 50%;
		top: 45%;
		transform: translate(-50%, -50%) rotate(-4deg);
		font-family: var(--font-display);
		font-size: clamp(8rem, 28vw, 22rem);
		line-height: 1;
		color: transparent;
		-webkit-text-stroke: 1px color-mix(in srgb, var(--accent) 12%, transparent);
		user-select: none;
	}

	.spotlight {
		position: absolute;
		inset: 0;
		background: radial-gradient(
			400px circle at var(--px, 50%) var(--py, 30%),
			color-mix(in srgb, var(--accent) 10%, transparent),
			transparent 70%
		);
	}

	.status-pill-wrap {
		margin-bottom: 18px;
	}

	.status-pill {
		display: inline-flex;
		align-items: center;
		gap: 8px;
		padding: 6px 14px;
		border-radius: 9999px;
		font-size: 0.8rem;
		font-weight: 600;
		background: color-mix(in srgb, var(--card) 80%, transparent);
		border: 1px solid var(--border);
	}

	.status-pill.online {
		border-color: #22c55e;
		color: #22c55e;
	}

	.dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: currentColor;
	}

	h1 {
		font-size: clamp(1.8rem, 6vw, 2.7rem);
		font-weight: 800;
		line-height: 1.15;
		max-width: 680px;
		margin: 0 auto;
		text-wrap: balance;
>>>>>>> Stashed changes
	}

	.sub {
		margin: 18px auto 0;
<<<<<<< Updated upstream
		max-width: 440px;
		color: var(--text-dim);
		font-size: 1rem;
	}

	.cta {
		margin: 32px auto 0;
		max-width: 320px;
	}

	.continue {
		display: inline-block;
		margin-top: 22px;
		font-size: 0.85rem;
		color: var(--text-dim);
	}

	.continue:hover {
		color: var(--accent);
	}

	.how {
		margin-top: 72px;
=======
		max-width: 520px;
		color: var(--text-dim);
		font-size: 1.05rem;
		line-height: 1.55;
	}

	.cta-row {
		display: flex;
		justify-content: center;
		gap: 14px;
		margin-top: 32px;
	}

	/* Showcase Section */
	.showcase-section {
		padding: 60px 0;
	}

	.section-header {
		text-align: center;
		margin-bottom: 48px;
	}

	.section-kicker {
		font-size: 0.75rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.12em;
		color: var(--accent);
		margin-bottom: 8px;
>>>>>>> Stashed changes
	}

	.max-w {
		max-width: 480px;
		margin: 8px auto 0;
	}

	.chat-showcase-grid {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 36px;
		align-items: center;
	}

	@media (max-width: 860px) {
		.chat-showcase-grid {
			grid-template-columns: 1fr;
		}
	}

	/* Phone Mockup */
	.phone-mockup {
		background: #0f172a;
		border: 2px solid color-mix(in srgb, var(--accent) 30%, #334155);
		border-radius: 28px;
		overflow: hidden;
		box-shadow: 0 20px 40px rgba(0, 0, 0, 0.4);
	}

	.phone-header {
		display: flex;
		align-items: center;
		gap: 12px;
		padding: 16px 20px;
		background: #1e293b;
		border-bottom: 1px solid #334155;
	}

<<<<<<< Updated upstream
	.how-card {
		animation-delay: calc(var(--i, 0) * 60ms);
	}

	.how-num {
		display: inline-flex;
=======
	.avatar {
		width: 40px;
		height: 40px;
		border-radius: 50%;
		background: var(--accent);
		display: flex;
>>>>>>> Stashed changes
		align-items: center;
		justify-content: center;
		font-size: 1.2rem;
	}

	.phone-title h4 {
		margin: 0;
		font-size: 0.95rem;
		color: #f8fafc;
	}

	.status-text {
		font-size: 0.72rem;
		color: #94a3b8;
	}

	.chat-body {
		padding: 20px;
		min-height: 380px;
		display: flex;
		flex-direction: column;
		gap: 14px;
		background: #0b1120;
	}

	.date-badge {
		align-self: center;
		background: #1e293b;
		padding: 4px 10px;
		border-radius: 6px;
		font-size: 0.68rem;
		color: #94a3b8;
		font-weight: 600;
	}

	.chat-bubble {
		max-width: 85%;
		padding: 10px 14px;
		border-radius: 14px;
		font-size: 0.88rem;
		line-height: 1.45;
		position: relative;
	}

	.chat-bubble.user {
		align-self: flex-end;
		background: #059669;
		color: #fff;
		border-bottom-right-radius: 4px;
	}

	.chat-bubble.bot {
		align-self: flex-start;
		background: #1e293b;
		color: #e2e8f0;
		border-bottom-left-radius: 4px;
		border: 1px solid #334155;
	}

	.chat-bubble.highlight {
		border-color: var(--accent);
		background: color-mix(in srgb, var(--accent) 15%, #1e293b);
	}

	.bubble-time {
		display: block;
		text-align: right;
		font-size: 0.65rem;
		color: rgba(255, 255, 255, 0.6);
		margin-top: 4px;
	}

	.chat-tabs {
		display: flex;
		border-top: 1px solid #334155;
		background: #1e293b;
	}

	.tab-btn {
		flex: 1;
		padding: 12px;
		background: transparent;
		border: none;
		color: #94a3b8;
		font-weight: 600;
		font-size: 0.85rem;
		cursor: pointer;
		transition: all 0.2s;
	}

	.tab-btn.active {
		color: #fff;
		background: color-mix(in srgb, var(--accent) 20%, transparent);
		border-bottom: 2px solid var(--accent);
	}

	/* Feature Cards */
	.feature-bullets {
		display: flex;
		flex-direction: column;
		gap: 16px;
	}

	.feature-card {
		display: flex;
		gap: 16px;
		padding: 20px;
	}

	.icon-wrap {
		font-size: 1.6rem;
		line-height: 1;
	}

	.feature-card h3 {
		margin: 0 0 6px 0;
		font-size: 1.05rem;
	}

	.feature-card p {
		margin: 0;
		font-size: 0.9rem;
	}

	/* Pairing Card */
	.pairing-section {
		padding: 40px 0;
	}

	.pairing-card {
		padding: 40px;
		background: radial-gradient(circle at top right, color-mix(in srgb, var(--accent) 8%, transparent), transparent 70%), var(--card);
	}

	.badge-tag {
		display: inline-block;
		font-size: 0.7rem;
		font-weight: 800;
		letter-spacing: 0.1em;
		padding: 3px 8px;
		border-radius: 4px;
		background: var(--accent);
		color: #000;
		margin-bottom: 12px;
	}

	.badge-tag.host {
		background: #eab308;
	}

	/* Host Section */
	.host-setup-section {
		padding: 50px 0;
	}

	.host-grid {
		display: grid;
		grid-template-columns: 1.2fr 1fr;
		gap: 36px;
		align-items: center;
	}

	@media (max-width: 860px) {
		.host-grid {
			grid-template-columns: 1fr;
		}
	}

	.setup-steps {
		display: flex;
		flex-direction: column;
		gap: 14px;
		margin: 24px 0;
	}

	.step-item {
		display: flex;
		align-items: center;
		gap: 14px;
	}

	.step-num {
		width: 28px;
		height: 28px;
		border-radius: 50%;
<<<<<<< Updated upstream
		background: var(--accent);
		border: 2px solid var(--accent-contrast);
		color: var(--accent-contrast);
		font-family: var(--font-mono);
		font-size: 0.75rem;
		font-weight: 800;
	}

	.how-card:nth-child(2) .how-num {
		background: var(--pop-pink);
	}

	.how-card:nth-child(3) .how-num {
		background: var(--pop-cyan);
	}

	.how-card h3 {
		margin: 8px 0 6px;
		font-size: 1rem;
	}

	.how-card p {
=======
		background: var(--card);
		border: 1px solid var(--border);
		display: flex;
		align-items: center;
		justify-content: center;
		font-weight: 700;
		font-size: 0.85rem;
		color: var(--accent);
	}

	.callout-box {
		padding: 14px 18px;
		background: var(--card);
		border-left: 3px solid #eab308;
		border-radius: 4px;
>>>>>>> Stashed changes
		font-size: 0.85rem;
	}

	/* QR Station */
	.qr-station {
		padding: 28px;
		text-align: center;
		display: flex;
		flex-direction: column;
		align-items: center;
	}

	.station-badge {
		margin-top: 6px;
		font-size: 0.78rem;
		font-weight: 700;
		padding: 4px 12px;
		border-radius: 9999px;
	}

	.station-badge.scan {
		background: rgba(234, 179, 8, 0.15);
		color: #eab308;
	}

	.qr-canvas-wrap {
		width: 240px;
		height: 240px;
		margin: 24px 0;
		display: flex;
		align-items: center;
		justify-content: center;
		background: #fff;
		border-radius: 12px;
		padding: 12px;
		box-shadow: 0 8px 24px rgba(0, 0, 0, 0.15);
	}

	.qr-success {
		color: #0f172a;
		padding: 10px;
	}

	.huge-icon {
		font-size: 3rem;
	}

	.station-meta {
		display: flex;
		justify-content: space-between;
		width: 100%;
		align-items: center;
		font-size: 0.8rem;
		color: var(--text-dim);
	}

	/* Commands Section */
	.commands-section {
		padding: 50px 0 80px 0;
	}

	.commands-grid {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 24px;
	}

	@media (max-width: 860px) {
		.commands-grid {
			grid-template-columns: 1fr;
		}
<<<<<<< Updated upstream
=======
	}

	.command-card {
		padding: 28px;
	}

	.command-card h3 {
		margin-top: 0;
		font-size: 1.15rem;
		border-bottom: 1px solid var(--border);
		padding-bottom: 12px;
		margin-bottom: 16px;
	}

	.cmd-list {
		list-style: none;
		padding: 0;
		margin: 0;
		display: flex;
		flex-direction: column;
		gap: 12px;
	}

	.cmd-list li {
		display: flex;
		flex-direction: column;
		gap: 4px;
		font-size: 0.88rem;
	}

	.cmd-list code {
		align-self: flex-start;
		background: var(--bg);
		padding: 2px 8px;
		border-radius: 4px;
		font-size: 0.85rem;
		font-weight: 700;
		color: var(--accent);
		border: 1px solid var(--border);
	}

	.cmd-list span {
		color: var(--text-dim);
		font-size: 0.82rem;
>>>>>>> Stashed changes
	}
</style>
