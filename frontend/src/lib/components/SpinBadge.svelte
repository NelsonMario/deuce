<script lang="ts">
	interface Props {
		text?: string;
		/** Seconds per rotation. */
		speed?: number;
	}

	let { text = 'FAIR ROTATION • RATING BALANCED • ', speed = 18 }: Props = $props();
</script>

<!--
	Rotating circular-text sticker — the classic awwwards hero ornament.
	Sits in a corner, spins slowly, and its center glyph is a minimal
	shuttlecock mark drawn inline (no emoji, no asset fetch).
-->
<div class="spin-badge" style="--spin-speed:{speed}s" aria-hidden="true">
	<svg viewBox="0 0 100 100" class="ring">
		<defs>
			<path id="badge-circle" d="M 50,50 m -36,0 a 36,36 0 1,1 72,0 a 36,36 0 1,1 -72,0" />
		</defs>
		<text>
			<textPath href="#badge-circle">{text}</textPath>
		</text>
	</svg>
	<svg viewBox="0 0 24 24" class="glyph">
		<!-- shuttlecock: cork + skirt -->
		<circle cx="12" cy="17.5" r="3" />
		<path d="M9.5 15 4.5 6.5l5-1.5L12 13Z" />
		<path d="M14.5 15l5-8.5-5-1.5L12 13Z" />
	</svg>
</div>

<style>
	.spin-badge {
		position: relative;
		width: 108px;
		height: 108px;
		display: grid;
		place-items: center;
		flex-shrink: 0;
	}

	.ring {
		position: absolute;
		inset: 0;
		animation: spin var(--spin-speed) linear infinite;
	}

	.ring text {
		font-family: var(--font-mono);
		font-size: 8.2px;
		font-weight: 700;
		letter-spacing: 0.32em;
		text-transform: uppercase;
		fill: var(--accent);
	}

	.glyph {
		width: 30px;
		height: 30px;
		fill: none;
		stroke: var(--pop-pink);
		stroke-width: 1.8;
		stroke-linejoin: round;
		stroke-linecap: round;
	}

	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.ring {
			animation: none;
		}
	}
</style>
