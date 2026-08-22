<script lang="ts">
	interface Props {
		items: string[];
		/** Seconds for one full loop. */
		speed?: number;
		separator?: string;
	}

	let { items, speed = 28, separator = '✦' }: Props = $props();
</script>

<!--
	Kinetic ticker: two identical sequences translated -50% in a loop, so the
	seam is invisible. Decorative only (aria-hidden) — screen readers already
	have the real content.
-->
<div class="marquee" style="--speed:{speed}s" aria-hidden="true">
	<div class="track">
		{#each [0, 1] as copy (copy)}
			<div class="seq">
				{#each items as item, i (i)}
					<span class="item">{item}</span>
					<span class="sep">{separator}</span>
				{/each}
			</div>
		{/each}
	</div>
</div>

<style>
	.marquee {
		overflow: hidden;
		user-select: none;
	}

	.track {
		display: flex;
		width: max-content;
		animation: scroll var(--speed) linear infinite;
	}

	.seq {
		display: flex;
		align-items: center;
		flex-shrink: 0;
	}

	.item {
		font-family: var(--font-display);
		font-size: clamp(1.1rem, 3vw, 1.6rem);
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: var(--text);
		padding: 0 18px;
		white-space: nowrap;
	}

	/* alternating outline style — every other word is hollow, a classic
	   poster trick that adds rhythm without extra color */
	.item:nth-child(4n + 3) {
		color: transparent;
		-webkit-text-stroke: 1px var(--text-dim);
	}

	.sep {
		color: var(--accent);
		font-size: 0.8em;
		transform: translateY(-1px);
	}

	@keyframes scroll {
		from {
			transform: translateX(0);
		}
		to {
			transform: translateX(-50%);
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.track {
			animation: none;
		}
	}
</style>
