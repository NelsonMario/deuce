<script lang="ts">
	import "../lib/styles/app.css";
	import ToastHost from "$lib/components/ToastHost.svelte";
	import Cursor from "$lib/components/Cursor.svelte";
	import Marquee from "$lib/components/Marquee.svelte";
	import { onMount } from "svelte";
	import { page } from "$app/state";

	let { children } = $props();

	let offline = $state(false);

	const ticker = [
		"Fair rotation",
		"Rating balanced",
		"No court politics",
		"Game on",
		"Deuce",
	];

	onMount(() => {
		offline = !navigator.onLine;
		const on = () => (offline = false);
		const off = () => (offline = true);
		window.addEventListener("online", on);
		window.addEventListener("offline", off);

		import("virtual:pwa-register")
			.then(({ registerSW }) => registerSW({ immediate: true }))
			.catch(() => {});

		return () => {
			window.removeEventListener("online", on);
			window.removeEventListener("offline", off);
		};
	});
</script>

<div class="shell">
	<div class="grain" aria-hidden="true"></div>
	<Cursor />

	<header class="topbar halftone">
		<div class="container-wide">
			<a href="/" class="brand">deuce<span class="brand-dot">.</span></a>
		</div>
	</header>

	{#if offline}
		<div class="offline-bar">
			You're offline — some actions won't go through until you're back.
		</div>
	{/if}

	<!-- keyed on the route so every navigation replays the entrance -->
	<main>
		{#key page.url.pathname}
			<div class="route-fade">
				{@render children()}
			</div>
		{/key}
	</main>

	<div class="ticker-wrap halftone">
		<Marquee items={ticker} />
	</div>

	<footer class="footer">
		<div class="container-wide footer-content">
			<div class="footer-brand">
				<span class="copyright"
					>© {new Date().getFullYear()} Nelson Mario</span
				>
			</div>
			<div class="creator-info">
				<div class="social-links">
					<a
						href="https://sonneil.space"
						target="_blank"
						rel="noopener noreferrer"
						class="social-link portfolio"
						aria-label="Portfolio (sonneil.space)"
						title="Portfolio (sonneil.space)"
					>
						<svg
							viewBox="0 0 24 24"
							width="18"
							height="18"
							fill="none"
							stroke="currentColor"
							stroke-width="2"
							stroke-linecap="round"
							stroke-linejoin="round"
						>
							<circle cx="12" cy="12" r="10" />
							<line x1="2" y1="12" x2="22" y2="12" />
							<path
								d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"
							/>
						</svg>
					</a>
					<a
						href="https://www.instagram.com/nlsnmario/"
						target="_blank"
						rel="noopener noreferrer"
						class="social-link instagram"
						aria-label="Instagram (@nlsnmario)"
						title="Instagram (@nlsnmario)"
					>
						<svg
							viewBox="0 0 24 24"
							width="18"
							height="18"
							fill="none"
							stroke="currentColor"
							stroke-width="2"
							stroke-linecap="round"
							stroke-linejoin="round"
						>
							<rect
								x="2"
								y="2"
								width="20"
								height="20"
								rx="5"
								ry="5"
							/>
							<path
								d="M16 11.37A4 4 0 1 1 12.63 8 4 4 0 0 1 16 11.37z"
							/>
							<line x1="17.5" y1="6.5" x2="17.51" y2="6.5" />
						</svg>
					</a>
					<a
						href="https://www.linkedin.com/in/nelsonmario/"
						target="_blank"
						rel="noopener noreferrer"
						class="social-link linkedin"
						aria-label="LinkedIn (Nelson Mario)"
						title="LinkedIn (Nelson Mario)"
					>
						<svg
							viewBox="0 0 24 24"
							width="18"
							height="18"
							fill="none"
							stroke="currentColor"
							stroke-width="2"
							stroke-linecap="round"
							stroke-linejoin="round"
						>
							<path
								d="M16 8a6 6 0 0 1 6 6v7h-4v-7a2 2 0 0 0-2-2 2 2 0 0 0-2 2v7h-4v-7a6 6 0 0 1 6-6z"
							/>
							<rect x="2" y="9" width="4" height="12" />
							<circle cx="4" cy="4" r="2" />
						</svg>
					</a>
				</div>
			</div>
		</div>
	</footer>
</div>

<ToastHost />

<style>
	.shell {
		min-height: 100vh;
		display: flex;
		flex-direction: column;
	}

	.topbar {
		position: sticky;
		top: 0;
		z-index: 50;
		backdrop-filter: blur(24px) saturate(180%);
		-webkit-backdrop-filter: blur(24px) saturate(180%);
		background: color-mix(in srgb, var(--bg) 78%, transparent);
		border-bottom: 1px solid var(--border-soft);
	}

	.topbar .container-wide {
		position: relative;
		padding-top: calc(12px + env(safe-area-inset-top, 0px));
		padding-bottom: 12px;
	}

	.brand {
		font-family: var(--font-display);
		font-weight: 700;
		font-size: 1.35rem;
		letter-spacing: -0.02em;
		display: inline-flex;
		align-items: baseline;
		gap: 1px;
		transition: opacity 0.18s ease;
	}

	.brand:hover {
		opacity: 0.75;
	}

	.brand-dot {
		color: var(--accent);
	}

	.offline-bar {
		background: var(--warn-bg);
		color: var(--warn);
		text-align: center;
		font-size: 0.82rem;
		font-weight: 600;
		padding: 8px 16px;
	}

	main {
		flex: 1;
		padding: 28px 0 calc(56px + env(safe-area-inset-bottom, 0px));
	}

	/* route entrance: quick fade + rise, replayed on every navigation.
	   Deliberately NO animation-fill-mode: `both` would leave the finished
	   keyframe's identity transform applied forever, making this div the
	   containing block for position:fixed descendants (host dock, HUD
	   panel, dialogs) — they'd anchor to the document instead of the
	   viewport and scroll away / get cropped. */
	.route-fade {
		animation: route-in 0.4s cubic-bezier(0.16, 1, 0.3, 1);
	}

	@keyframes route-in {
		from {
			opacity: 0;
			transform: translateY(12px) scale(0.995);
		}
		to {
			opacity: 1;
			transform: translateY(0) scale(1);
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.route-fade {
			animation: none;
		}
	}

	.ticker-wrap {
		border-top: 1px solid var(--border-soft);
		background: color-mix(in srgb, var(--bg-elevated) 55%, transparent);
		padding: 10px 0;
		color: var(--text-dim);
	}

	.footer {
		margin-top: auto;
		border-top: 1px solid var(--border-soft);
		background: var(--bg-elevated);
		padding: 20px 0 calc(20px + env(safe-area-inset-bottom, 0px));
	}

	.footer-content {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 16px;
		flex-wrap: wrap;
	}

	.footer-brand {
		display: flex;
		align-items: center;
		gap: 10px;
	}

	.creator-info {
		display: flex;
		align-items: center;
		gap: 12px;
		font-size: 0.85rem;
		color: var(--text-dim);
	}

	.copyright {
		font-size: 0.8rem;
		color: var(--text-dim);
	}

	.social-links {
		display: flex;
		align-items: center;
		gap: 8px;
	}

	.social-link {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 34px;
		height: 34px;
		border-radius: var(--radius-sm);
		border: 1.5px solid var(--border);
		background: var(--bg);
		color: var(--text-dim);
		transition: all 0.15s ease;
	}

	.social-link:hover {
		color: var(--accent-contrast);
		background: var(--accent);
		border-color: var(--accent);
		transform: translateY(-2px);
		box-shadow: 0 4px 14px 0 color-mix(in srgb, var(--accent) 35%, transparent);
	}

	.social-link.instagram:hover {
		background: var(--pop-pink);
		color: #fff;
		border-color: var(--pop-pink);
	}

	.social-link.linkedin:hover {
		background: var(--pop-cyan);
		color: #042436;
		border-color: var(--pop-cyan);
	}
</style>
