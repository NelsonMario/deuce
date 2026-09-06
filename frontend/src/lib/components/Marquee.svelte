<script lang="ts">
	interface Props {
		items: string[];
		/** Seconds for one full loop. */
		speed?: number;
		separator?: string;
	}

	let { items, speed = 28, separator = '·' }: Props = $props();
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
		-webkit-mask-image: linear-gradient(90deg, transparent, #000 8%, #000 92%, transparent);
		mask-image: linear-gradient(90deg, transparent, #000 8%, #000 92%, transparent);
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
		font-size: 0.95rem;
		letter-spacing: 0.02em;
		color: var(--text-dim);
		padding: 0 16px;
		white-space: nowrap;
	}

	.sep {
		color: var(--accent);
		font-size: 0.75rem;
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
