<script lang="ts">
	import { toasts, toast } from '../stores/toast';
</script>

<div class="toast-host" role="status" aria-live="polite">
	{#each $toasts as t (t.id)}
		<button type="button" class="toast toast-{t.kind}" onclick={() => toast.dismiss(t.id)}>
			{t.message}
		</button>
	{/each}
</div>

<style>
	.toast-host {
		position: fixed;
		left: 0;
		right: 0;
		bottom: max(16px, env(safe-area-inset-bottom));
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 8px;
		padding: 0 16px;
		z-index: 200;
		pointer-events: none;
	}

	.toast {
		pointer-events: auto;
		max-width: 420px;
		width: 100%;
		text-align: center;
		font-family: inherit;
		font-size: 0.88rem;
		font-weight: 500;
		padding: 12px 16px;
		border-radius: 12px;
		background: var(--bg-elevated-2);
		border: 1px solid var(--border);
		color: var(--text);
		box-shadow: var(--shadow);
		cursor: pointer;
		animation: toast-in 0.25s cubic-bezier(0.16, 1, 0.3, 1) both;
	}

	.toast-success {
		border-color: color-mix(in srgb, var(--accent) 40%, var(--border));
		color: var(--accent);
	}

	.toast-error {
		border-color: color-mix(in srgb, var(--danger) 40%, var(--border));
		color: var(--danger);
	}

	@keyframes toast-in {
		from {
			opacity: 0;
			transform: translateY(8px) scale(0.98);
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
