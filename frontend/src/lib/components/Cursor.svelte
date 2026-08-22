<script lang="ts">
	import { onMount } from 'svelte';

	/**
	 * Awwwards-style custom cursor: an instant dot plus a lerped trailing
	 * ring that dilates over interactive elements and compresses on click.
	 * Enabled only for fine pointers (mouse/trackpad) without reduced-motion;
	 * touch devices keep native behavior untouched.
	 */
	let enabled = $state(false);
	let dotEl: HTMLDivElement | undefined = $state();
	let ringEl: HTMLDivElement | undefined = $state();

	onMount(() => {
		const fine = window.matchMedia('(pointer: fine)').matches;
		const reduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
		if (!fine || reduced) return;
		enabled = true;
		document.documentElement.classList.add('custom-cursor');

		const pos = { x: innerWidth / 2, y: innerHeight / 2 };
		const ring = { x: pos.x, y: pos.y };
		let scale = 1;
		let targetScale = 1;
		let raf = 0;

		const INTERACTIVE = 'a, button, [role="tab"], [role="button"], input, select, textarea, label, summary, [data-cursor]';

		function onMove(e: PointerEvent) {
			pos.x = e.clientX;
			pos.y = e.clientY;
			if (dotEl) dotEl.style.transform = `translate(${pos.x}px, ${pos.y}px) translate(-50%, -50%)`;
			targetScale = (e.target as Element | null)?.closest?.(INTERACTIVE) ? 2.1 : 1;
		}

		function onDown() {
			targetScale = 0.7;
		}

		function onUp(e: PointerEvent) {
			targetScale = (e.target as Element | null)?.closest?.(INTERACTIVE) ? 2.1 : 1;
		}

		function tick() {
			ring.x += (pos.x - ring.x) * 0.16;
			ring.y += (pos.y - ring.y) * 0.16;
			scale += (targetScale - scale) * 0.18;
			if (ringEl)
				ringEl.style.transform = `translate(${ring.x}px, ${ring.y}px) translate(-50%, -50%) scale(${scale})`;
			raf = requestAnimationFrame(tick);
		}

		window.addEventListener('pointermove', onMove, { passive: true });
		window.addEventListener('pointerdown', onDown);
		window.addEventListener('pointerup', onUp);
		raf = requestAnimationFrame(tick);

		return () => {
			document.documentElement.classList.remove('custom-cursor');
			cancelAnimationFrame(raf);
			window.removeEventListener('pointermove', onMove);
			window.removeEventListener('pointerdown', onDown);
			window.removeEventListener('pointerup', onUp);
		};
	});
</script>

{#if enabled}
	<div class="cursor-ring" bind:this={ringEl} aria-hidden="true"></div>
	<div class="cursor-dot" bind:this={dotEl} aria-hidden="true"></div>
{/if}

<style>
	.cursor-dot,
	.cursor-ring {
		position: fixed;
		top: 0;
		left: 0;
		z-index: 400;
		pointer-events: none;
		border-radius: 50%;
		will-change: transform;
	}

	/* difference blending keeps the cursor visible over every surface — it
	   inverts whatever is underneath (lime over lime turns dark, etc.). */
	.cursor-dot {
		width: 6px;
		height: 6px;
		background: var(--accent);
		mix-blend-mode: difference;
	}

	.cursor-ring {
		width: 34px;
		height: 34px;
		border: 1.5px solid color-mix(in srgb, var(--accent) 80%, white);
		mix-blend-mode: difference;
	}
</style>
