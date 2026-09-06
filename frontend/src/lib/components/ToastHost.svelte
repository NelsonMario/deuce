<script lang="ts">
	import { toasts, toast } from '../stores/toast';
</script>

<div class="toast-host" role="status" aria-live="polite">
	{#each $toasts as t (t.id)}
		<button
			type="button"
			class="toast toast-{t.kind}"
			onclick={() => toast.dismiss(t.id)}
		>
			<span class="toast-dot" aria-hidden="true"></span>
			<span class="toast-msg">{t.message}</span>
		</button>
	{/each}
</div>

<style>
	.toast-host {
		position: fixed;
		z-index: 200;
		top: calc(10px + env(safe-area-inset-top, 0px));
		left: 0;
		right: 0;
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 8px;
		padding: 0 16px;
		pointer-events: none;
	}

	.toast {
		pointer-events: auto;
		max-width: 430px;
		width: 100%;
		display: flex;
		align-items: center;
		gap: 10px;
		text-align: left;
		font-family: inherit;
		font-size: 0.92rem;
		font-weight: 500;
		line-height: 1.35;
		color: var(--text);
		padding: 13px 16px;
		border-radius: 16px;
		background: rgba(28, 28, 32, 0.86);
		backdrop-filter: blur(24px) saturate(180%);
		-webkit-backdrop-filter: blur(24px) saturate(180%);
		border: 1px solid rgba(255, 255, 255, 0.12);
		box-shadow:
			0 12px 32px 0 rgba(0, 0, 0, 0.45),
			0 2px 6px 0 rgba(0, 0, 0, 0.3),
			inset 0 1px 0 0 rgba(255, 255, 255, 0.12);
		cursor: pointer;
		animation: toast-in 0.32s var(--ease-spring) both;
	}

	.toast-dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--text-dim);
		flex-shrink: 0;
	}

	.toast-success .toast-dot {
		background: var(--ok, #30d158);
	}

	.toast-error .toast-dot {
		background: var(--danger);
	}

	.toast-info .toast-dot {
		background: var(--pop-cyan);
	}

	@keyframes toast-in {
		from {
			opacity: 0;
			transform: translateY(-14px) scale(0.97);
		}
		to {
			opacity: 1;
			transform: translateY(0) scale(1);
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.toast {
			animation: none;
		}
	}
</style>