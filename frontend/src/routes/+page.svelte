<script lang="ts">
	import Typewriter from "$lib/components/Typewriter.svelte";
	import { identity } from "$lib/stores/identity";

	const phrases = [
		"Stop picking your favorite partner. We’re watching. 👀",
		"No more “you played twice already.”",
		"Your friends can’t save your rating.",
		"Diinjak-injak? Saya DIAM!, Dimaki-maki? Saya DIAM!. Kali ini... SAYA AKAN LAWAN!!!",
		"Think you deserve the next game? Prove it.",
	];

	let lastSessionId = $derived($identity.lastSessionId);
</script>

<svelte:head>
	<title>deuce — court rotation</title>
</svelte:head>

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
		</div>
	</div>
</section>

<style>
	.hero {
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
	}

	.how-grid {
		display: grid;
		grid-template-columns: repeat(3, 1fr);
		gap: 12px;
	}

	.how-card {
		animation-delay: calc(var(--i, 0) * 60ms);
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
		font-size: 0.85rem;
	}

	@media (max-width: 640px) {
		.how-grid {
			grid-template-columns: 1fr;
		}
	}
</style>
