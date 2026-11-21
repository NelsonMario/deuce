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
			<nav class="topnav" aria-label="Primary">
				<a href="/#quick-start">Quick start</a>
				<a href="/setup">Bot setup</a>
				<a href="/#commands">Commands</a>
				<a href="/#troubleshooting">Troubleshooting</a>
			</nav>
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
		z-index: 20;
		background: color-mix(in srgb, var(--bg) 82%, transparent);
		backdrop-filter: blur(14px) saturate(140%);
		-webkit-backdrop-filter: blur(14px) saturate(140%);
		border-bottom: 1px solid var(--border-soft);
	}

	.topbar-inner {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 16px;
		padding-top: 14px;
		padding-bottom: 14px;
	}

	.brand {
		font-weight: 800;
		font-size: 1.35rem;
		letter-spacing: -0.03em;
		color: var(--text);
		text-decoration: none;
	}

	.brand-dot {
		color: var(--accent);
	}

	.topnav {
		display: flex;
		gap: 18px;
		font-size: 0.9rem;
		color: var(--text-dim);
	}

	.topnav a:hover {
		color: var(--text);
	}

	main {
		flex: 1;
		padding: 28px 0 72px;
	}

	.footer {
		border-top: 1px solid var(--border-soft);
		padding: 20px 0;
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

	.footer-links a:hover {
		color: var(--accent);
	}

	@media (max-width: 640px) {
		.topnav { display: none; }
	}
</style>
