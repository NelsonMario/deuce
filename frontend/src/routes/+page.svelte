<script lang="ts">
	import Typewriter from "$lib/components/Typewriter.svelte";
	import Marquee from "$lib/components/Marquee.svelte";
	import SpinBadge from "$lib/components/SpinBadge.svelte";
	import { identity } from "$lib/stores/identity";
	import { reveal, tilt, magnetic, trackPointer } from "$lib/actions/motion";

	const phrases = [
		"Stop picking your favorite partner. We’re watching. 👀",
		"No more “you played twice already.”",
		"Your friends can’t save your rating.",
		"Diinjak-injak? Saya DIAM!, Dimaki-maki? Saya DIAM!. Kali ini... SAYA AKAN LAWAN!!!",
		"Think you deserve the next game? Prove it.",
	];

	const ticker = [
		"Host creates a club",
		"Players join by code",
		"The engine picks fair fours",
		"Ratings settle it",
	];

	// Sessions end and go stale, so "continue" points at the club (a
	// permanent home), never at a stored session ID. lastClubId follows the
	// club this device most recently created/visited/joined; the fallbacks
	// cover older storage that never recorded it.
	let lastClubId = $derived(
		$identity.lastClubId ??
			Object.keys($identity.clubs)[0] ??
			Object.keys($identity.clubMemberships)[0]
	);

	let heroEl: HTMLElement | undefined = $state();
</script>

<svelte:head>
	<title>deuce — court rotation</title>
</svelte:head>

<!-- Full-bleed band so the halftone texture + watermark span the whole
     viewport; the .container inside keeps the copy at reading width. -->
<section class="hero-band halftone rise-in" bind:this={heroEl} use:trackPointer>
	<!-- pointer-following spotlight + oversized outline watermark -->
	<div class="hero-fx" aria-hidden="true">
		<span class="watermark">DEUCE</span>
		<div class="spotlight"></div>
	</div>

	<div class="container hero">
		<div class="spin-slot" use:reveal={{ delay: 500 }}>
			<SpinBadge />
		</div>

		<p class="kicker" use:reveal={{ delay: 60 }}>
			Smart(?) Court Rotation
		</p>
		<h1 use:reveal={{ delay: 140 }}>
			<Typewriter {phrases} />
		</h1>
		<p class="sub" use:reveal={{ delay: 240 }}>
			Build your club. Bring your crew. We’ll handle the rotation.
		</p>

		<div class="cta stack" use:reveal={{ delay: 340 }}>
			<a href="/host" class="btn btn-primary btn-block magnetic" use:magnetic>
				<span>Host a session</span>
				<svg
					viewBox="0 0 24 24"
					width="16"
					height="16"
					fill="none"
					stroke="currentColor"
					stroke-width="2.6"
					stroke-linecap="round"
					stroke-linejoin="round"><path d="M5 12h14M13 6l6 6-6 6" /></svg
				>
			</a>
			<a href="/join" class="btn btn-ghost btn-block">Join with a code</a>
		</div>

		{#if lastClubId}
			<a href="/club/{lastClubId}" class="continue" use:reveal={{ delay: 440 }}>
				↳ Back to your club
			</a>
		{/if}
	</div>
</section>

<div class="ticker-band rise-in">
	<Marquee items={ticker} speed={34} />
</div>

<section class="container-wide how">
	<h2 class="how-title" use:reveal>How it works</h2>
	<div class="how-grid">
		<!-- reveal on a wrapper: reveal and tilt each own `transform`, so
		     they must live on separate elements -->
		<div class="cell" use:reveal={{ delay: 0 }}>
			<div class="how-card card card-tight tiltable" use:tilt={{ max: 7 }}>
				<span class="how-num">01</span>
				<h3>Create</h3>
				<p class="muted">
					Host makes a club and a session, gets a join code to share on
					the spot.
				</p>
			</div>
		</div>
		<div class="cell" use:reveal={{ delay: 110 }}>
			<div class="how-card card card-tight tiltable" use:tilt={{ max: 7 }}>
				<span class="how-num">02</span>
				<h3>Join</h3>
				<p class="muted">
					Players scan or type the code, add their name, and land in the
					queue.
				</p>
			</div>
		</div>
		<div class="cell" use:reveal={{ delay: 220 }}>
			<div class="how-card card card-tight tiltable" use:tilt={{ max: 7 }}>
				<span class="how-num">03</span>
				<h3>Play</h3>
				<p class="muted">
					Matches are generated from ratings and wait time. Scores feed
					the next one.
				</p>
			</div>
		</div>
	</div>
</section>

<style>
	.hero-band {
		position: relative;
		overflow: hidden;
	}

	.hero {
		position: relative;
		padding-top: 8vh;
		padding-bottom: 9vh;
		text-align: center;
	}

	/* ---- layered hero fx ---- */

	.hero-fx {
		position: absolute;
		inset: 0;
		z-index: -1;
		pointer-events: none;
	}

	/* giant hollow watermark, slightly rotated for poster energy */
	.watermark {
		position: absolute;
		left: 50%;
		top: 42%;
		transform: translate(-50%, -50%) rotate(-4deg);
		font-family: var(--font-display);
		font-size: clamp(9rem, 34vw, 26rem);
		line-height: 1;
		letter-spacing: 0.04em;
		color: transparent;
		-webkit-text-stroke: 1px color-mix(in srgb, var(--accent) 14%, transparent);
		user-select: none;
		animation: drift 14s ease-in-out infinite alternate;
	}

	@keyframes drift {
		from {
			transform: translate(-52%, -52%) rotate(-5deg);
		}
		to {
			transform: translate(-48%, -46%) rotate(-2.5deg);
		}
	}

	/* cursor spotlight — position driven by --px/--py via trackPointer */
	.spotlight {
		position: absolute;
		inset: 0;
		background: radial-gradient(
			340px circle at var(--px, 50%) var(--py, 30%),
			color-mix(in srgb, var(--accent) 9%, transparent),
			transparent 70%
		);
	}

	@media (prefers-reduced-motion: reduce) {
		.watermark {
			animation: none;
		}
	}

	.spin-slot {
		display: flex;
		justify-content: center;
		margin-bottom: 18px;
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
		position: relative;
		font-size: clamp(1.9rem, 6vw, 2.6rem);
		font-weight: 700;
		letter-spacing: -0.02em;
		line-height: 1.15;
		min-height: 2.4em;
		max-width: 620px;
		margin: 0 auto;
		text-wrap: balance;
	}

	.sub {
		margin: 18px auto 0;
		max-width: 440px;
		color: var(--text-dim);
		font-size: 1rem;
	}

	.cta {
		margin: 32px auto 0;
		max-width: 320px;
	}

	.magnetic {
		gap: 10px;
	}

	.magnetic svg {
		transition: transform 0.25s cubic-bezier(0.16, 1, 0.3, 1);
	}

	.magnetic:hover svg {
		transform: translateX(5px);
	}

	.continue {
		display: inline-block;
		margin-top: 22px;
		font-size: 0.85rem;
		color: var(--text-dim);
		transition: color 0.15s ease, letter-spacing 0.2s ease;
	}

	.continue:hover {
		color: var(--accent);
		letter-spacing: 0.03em;
	}

	/* ---- ticker band ---- */

	.ticker-band {
		border-block: 2px solid var(--border);
		background: color-mix(in srgb, var(--accent) 4%, var(--bg));
		padding: 12px 0;
		color: var(--accent);
	}

	/* ---- how it works ---- */

	.how {
		margin-top: 64px;
	}

	.how-title {
		font-family: var(--font-display);
		font-weight: 400;
		font-size: clamp(1.6rem, 4vw, 2.2rem);
		letter-spacing: 0.03em;
		text-transform: uppercase;
		margin-bottom: 18px;
	}

	.how-grid {
		display: grid;
		grid-template-columns: repeat(3, 1fr);
		gap: 12px;
	}

	.cell {
		display: grid;
	}

	.how-card {
		border-radius: var(--radius-lg);
		height: 100%;
	}

	.how-num {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 28px;
		height: 28px;
		border-radius: 50%;
		background: var(--accent);
		border: 2px solid var(--accent-contrast);
		color: var(--accent-contrast);
		font-family: var(--font-mono);
		font-size: 0.75rem;
		font-weight: 800;
		transition: transform 0.25s cubic-bezier(0.16, 1, 0.3, 1);
	}

	.how-card:hover .how-num {
		transform: rotate(-12deg) scale(1.15);
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
		font-weight: 700;
	}

	.how-card p {
		font-size: 0.85rem;
	}

	@media (max-width: 640px) {
		.how-grid {
			grid-template-columns: 1fr;
		}

		.spin-slot {
			display: none;
		}
	}
</style>
