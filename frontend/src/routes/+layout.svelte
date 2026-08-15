<script lang="ts">
	import '../lib/styles/app.css';
	import ToastHost from '$lib/components/ToastHost.svelte';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';

	let { children } = $props();

	let offline = $state(false);

	function goBack() {
		// A deep-linked/cold-loaded page has no in-app history to pop, so
		// falling back to home avoids "Back" stranding the visitor outside
		// the app (or on an unrelated site they arrived from).
		if (typeof window !== 'undefined' && window.history.length > 1) {
			window.history.back();
		} else {
			goto('/');
		}
	}

	onMount(() => {
		offline = !navigator.onLine;
		const on = () => (offline = false);
		const off = () => (offline = true);
		window.addEventListener('online', on);
		window.addEventListener('offline', off);

		import('virtual:pwa-register')
			.then(({ registerSW }) => registerSW({ immediate: true }))
			.catch(() => {});

		return () => {
			window.removeEventListener('online', on);
			window.removeEventListener('offline', off);
		};
	});
</script>

<div class="shell">
	<header class="topbar halftone">
		<div class="container-wide row spread">
			<a href="/" class="brand">deuce<span class="brand-dot">.</span></a>
			{#if page.url.pathname !== '/'}
				<button type="button" class="back-btn" onclick={goBack} aria-label="Back">
					<svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
						<path d="M15 5 8 12l7 7" />
					</svg>
				</button>
			{/if}
		</div>
	</header>

	{#if offline}
		<div class="offline-bar">You're offline — some actions won't go through until you're back.</div>
	{/if}

	<main>
		{@render children()}
	</main>

	<footer class="footer">
		<div class="container-wide">v{__APP_VERSION__}</div>
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
		backdrop-filter: blur(10px);
		background: color-mix(in srgb, var(--bg) 82%, transparent);
		border-bottom: 2px solid var(--border);
		overflow: hidden;
	}

	.topbar .container-wide {
		position: relative;
		padding-top: 14px;
		padding-bottom: 14px;
	}

	.brand {
		font-family: var(--font-display);
		font-weight: 400;
		font-size: 1.5rem;
		letter-spacing: 0.02em;
		text-transform: uppercase;
		/* pop-art "print offset" — a duplicate flat-color layer nudged behind
		   the text, like a slightly misregistered comic print. */
		text-shadow: 2px 2px 0 var(--pop-pink);
	}

	.brand-dot {
		color: var(--accent);
		text-shadow: none;
	}

	.back-btn {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 36px;
		height: 36px;
		border: 2px solid var(--border);
		border-radius: 100px;
		background: var(--bg-elevated);
		color: var(--text);
		cursor: pointer;
	}

	.back-btn:hover {
		border-color: var(--accent);
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
		padding: 32px 0 64px;
	}

	.footer {
		padding: 12px 0 20px;
		text-align: center;
		font-size: 0.72rem;
		color: var(--text-faint);
	}
</style>
