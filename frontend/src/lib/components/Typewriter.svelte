<script lang="ts">
	interface Props {
		phrases: string[];
		typeSpeed?: number;
		deleteSpeed?: number;
		pause?: number;
		loop?: boolean;
		class?: string;
	}

	let {
		phrases,
		typeSpeed = 42,
		deleteSpeed = 22,
		pause = 1900,
		loop = true,
		class: className = ''
	}: Props = $props();

	let display = $state('');
	let reduced = $state(false);

	function wait(ms: number, signal: { cancelled: boolean }) {
		return new Promise<void>((resolve) => {
			const id = setTimeout(resolve, ms);
			if (signal.cancelled) clearTimeout(id);
		});
	}

	$effect(() => {
		reduced =
			typeof window !== 'undefined' &&
			window.matchMedia('(prefers-reduced-motion: reduce)').matches;

		if (reduced) {
			display = phrases[0] ?? '';
			return;
		}

		const state = { cancelled: false };

		(async () => {
			let phraseIndex = 0;
			while (!state.cancelled) {
				const phrase = phrases[phraseIndex % phrases.length];
				for (let i = 0; i <= phrase.length; i++) {
					if (state.cancelled) return;
					display = phrase.slice(0, i);
					await wait(typeSpeed, state);
				}
				const isLast = !loop && phraseIndex >= phrases.length - 1;
				if (isLast) return;
				await wait(pause, state);
				for (let i = phrase.length; i >= 0; i--) {
					if (state.cancelled) return;
					display = phrase.slice(0, i);
					await wait(deleteSpeed, state);
				}
				phraseIndex += 1;
			}
		})();

		return () => {
			state.cancelled = true;
		};
	});
</script>

<span class="typewriter {className}">
	<span aria-hidden="true">{display}<span class="caret"></span></span>
	<span class="sr-only">{phrases[0]}</span>
</span>

<style>
	.typewriter {
		display: inline-block;
	}
	.caret {
		display: inline-block;
		width: 0.09em;
		margin-left: 0.06em;
		height: 1em;
		vertical-align: -0.12em;
		background: currentColor;
		animation: blink 1s step-end infinite;
	}
	@keyframes blink {
		0%,
		49% {
			opacity: 1;
		}
		50%,
		100% {
			opacity: 0;
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.caret {
			animation: none;
			opacity: 0;
		}
	}
	.sr-only {
		position: absolute;
		width: 1px;
		height: 1px;
		overflow: hidden;
		clip: rect(0, 0, 0, 0);
	}
</style>
