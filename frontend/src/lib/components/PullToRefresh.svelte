<script lang="ts">
	import { fade } from 'svelte/transition';

	let { onrefresh, children }: { onrefresh: () => Promise<void>; children: import('svelte').Snippet } =
		$props();

	// Icon-only pull-to-refresh, Instagram-style: no "Refresh" button anywhere,
	// the gesture itself is the affordance. Text would need translation and
	// adds noise; a rotating arrow communicates pull/release/loading on its own.
	let refreshing = $state(false);
	let pullStartY = $state<number | null>(null);
	let pullDistance = $state(0);
	let pullReady = $derived(pullDistance > 74);
	let pullVisible = $derived(refreshing || pullDistance > 0);
	let pullAngle = $derived(Math.min(180, (pullDistance / 74) * 180));

	function onTouchStart(e: TouchEvent) {
		if (window.scrollY > 0 || refreshing) return;
		pullStartY = e.touches[0]?.clientY ?? null;
	}

	function onTouchMove(e: TouchEvent) {
		if (pullStartY === null || refreshing) return;
		const y = e.touches[0]?.clientY ?? pullStartY;
		const delta = y - pullStartY;
		pullDistance = delta > 0 ? Math.min(96, delta * 0.45) : 0;
	}

	async function onTouchEnd() {
		if (pullReady) {
			refreshing = true;
			try {
				await onrefresh();
			} finally {
				refreshing = false;
			}
		}
		pullStartY = null;
		pullDistance = 0;
	}
</script>

<svelte:window ontouchstart={onTouchStart} ontouchmove={onTouchMove} ontouchend={onTouchEnd} ontouchcancel={onTouchEnd} />

{#if pullVisible}
	<div class="pull-indicator" class:ready={pullReady} transition:fade={{ duration: 140 }}>
		<svg
			class="pull-icon"
			class:spin={refreshing}
			style={refreshing ? '' : `transform: rotate(${pullAngle}deg);`}
			width="18"
			height="18"
			viewBox="0 0 24 24"
			fill="none"
			stroke="currentColor"
			stroke-width="2.5"
			stroke-linecap="round"
			stroke-linejoin="round"
		>
			<path d="M12 5v14M5 12l7 7 7-7" />
		</svg>
	</div>
{/if}

{@render children()}

<style>
	.pull-indicator {
		position: fixed;
		top: calc(10px + env(safe-area-inset-top, 0px));
		left: 50%;
		z-index: 90;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 40px;
		height: 40px;
		border: 1.5px solid var(--border);
		border-radius: 50%;
		background: color-mix(in srgb, var(--bg-elevated) 92%, transparent);
		color: var(--text-dim);
		box-shadow: var(--shadow-sm);
		transform: translateX(-50%);
		backdrop-filter: blur(8px);
	}

	.pull-indicator.ready {
		background: var(--border);
		color: var(--accent-contrast);
	}

	.pull-icon {
		transition: transform 0.08s ease;
	}

	.pull-icon.spin {
		animation: pull-spin 0.7s linear infinite;
	}

	@keyframes pull-spin {
		to {
			transform: rotate(360deg);
		}
	}
</style>
