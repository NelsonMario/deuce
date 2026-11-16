<script lang="ts">
	import "../lib/styles/app.css";
	import { onMount } from "svelte";

	let { children } = $props();

	onMount(() => {
		import("virtual:pwa-register")
			.then(({ registerSW }) => registerSW({ immediate: true }))
			.catch(() => {});
	});
</script>

<div class="shell">
	<header class="topbar">
		<div class="container-wide topbar-inner">
			<a href="/" class="brand">deuce<span class="brand-dot">.</span></a>
			<span class="topbar-label">Documentation</span>
		</div>
	</header>

	<main>
		{@render children()}
	</main>

	<footer class="footer">
		<div class="container-wide footer-inner">
			<span>© {new Date().getFullYear()} Nelson Mario</span>
			<span class="footer-links">
				<a href="https://sonneil.space" target="_blank" rel="noopener noreferrer">Website</a>
				<a href="https://www.instagram.com/nlsnmario/" target="_blank" rel="noopener noreferrer">Instagram</a>
				<a href="https://www.linkedin.com/in/nelsonmario/" target="_blank" rel="noopener noreferrer">LinkedIn</a>
			</span>
		</div>
	</footer>
</div>

<style>
	.shell {
		min-height: 100vh;
		display: flex;
		flex-direction: column;
	}

	.topbar {
		position: sticky;
		top: 0;
		z-index: 10;
		background: var(--bg);
		border-bottom: 1px solid var(--border);
	}

	.topbar-inner {
		display: flex;
		align-items: baseline;
		gap: 12px;
		padding-top: 14px;
		padding-bottom: 14px;
	}

	.brand {
		font-weight: 700;
		font-size: 1.25rem;
		color: var(--text);
		text-decoration: none;
	}

	.brand-dot {
		color: var(--accent);
	}

	.topbar-label {
		color: var(--text-dim);
		font-size: 0.9rem;
	}

	main {
		flex: 1;
		padding: 32px 0 64px;
	}

	.footer {
		border-top: 1px solid var(--border);
		padding: 16px 0;
		font-size: 0.85rem;
		color: var(--text-dim);
	}

	.footer-inner {
		display: flex;
		justify-content: space-between;
		flex-wrap: wrap;
		gap: 8px 16px;
	}

	.footer-links {
		display: flex;
		gap: 16px;
	}

	.footer-links a {
		color: var(--text-dim);
	}

	.footer-links a:hover {
		color: var(--accent);
	}
</style>
