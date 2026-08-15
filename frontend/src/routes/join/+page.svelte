<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { api, ApiError } from '$lib/api';
	import { identity } from '$lib/stores/identity';
	import { toast } from '$lib/stores/toast';
	import type { Gender } from '$lib/types';

	const UUID_RE = /[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/i;

	let sessionLink = $state(page.url.searchParams.get('session') ?? '');
	let joinCode = $state(page.url.searchParams.get('code') ?? '');
	let displayName = $state('');
	let gender = $state<Gender>('MALE');
	let submitting = $state(false);

	let sessionId = $derived(sessionLink.match(UUID_RE)?.[0] ?? '');

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		if (!sessionId || !joinCode.trim() || !displayName.trim()) return;
		submitting = true;
		try {
			const { session, session_player, you } = await api.joinSession(sessionId, {
				join_code: joinCode.trim(),
				display_name: displayName.trim(),
				gender
			});
			identity.rememberSession(session.id, {
				clubId: session.club_id,
				token: you.token,
				sessionPlayerId: session_player.id,
				player: you.player
			});
			if (you.returning) {
				const rating = await api.getPlayerRating(you.player.id, you.token).catch(() => null);
				toast.success(
					rating
						? `Welcome back, ${you.player.display_name} — rating ${Math.round(rating.rating)}.`
						: `Welcome back, ${you.player.display_name}.`
				);
			} else {
				toast.success(`You're in, ${you.player.display_name}.`);
			}
			goto(`/session/${session.id}`);
		} catch (err) {
			toast.error(err instanceof ApiError ? err.message : "Couldn't join that session.");
		} finally {
			submitting = false;
		}
	}
</script>

<svelte:head>
	<title>Join a session — deuce</title>
</svelte:head>

<section class="container rise-in">
	<h1>Join a session</h1>
	<p class="muted lead">Paste the link your host sent, or type the session ID and join code.</p>

	<form onsubmit={submit} class="card stack">
		<div class="field">
			<label for="link">Session link or ID</label>
			<input
				id="link"
				class="input"
				placeholder="https://…/join?session=… or the ID"
				bind:value={sessionLink}
				autocomplete="off"
				required
			/>
			{#if sessionLink && !sessionId}
				<span class="hint hint-warn">Doesn't look like a valid session link yet.</span>
			{/if}
		</div>

		<div class="field">
			<label for="code">Join code</label>
			<input
				id="code"
				class="input mono"
				placeholder="e.g. 4K9P2Q"
				bind:value={joinCode}
				autocomplete="off"
				style="letter-spacing:0.08em; text-transform:uppercase;"
				required
			/>
		</div>

		<div class="field">
			<label for="name">Your name</label>
			<input
				id="name"
				class="input"
				placeholder="How others will see you"
				bind:value={displayName}
				maxlength="60"
				autocomplete="off"
				required
			/>
		</div>

		<div class="field">
			<label for="gender">Gender</label>
			<div class="segmented" role="radiogroup" aria-label="Gender">
				<button type="button" class:active={gender === 'MALE'} onclick={() => (gender = 'MALE')}>
					Male
				</button>
				<button
					type="button"
					class:active={gender === 'FEMALE'}
					onclick={() => (gender = 'FEMALE')}
				>
					Female
				</button>
			</div>
		</div>

		<button type="submit" class="btn btn-primary btn-block" disabled={submitting || !sessionId}>
			{submitting ? 'Joining…' : 'Join session'}
		</button>
	</form>
</section>

<style>
	h1 {
		font-size: 1.7rem;
		font-weight: 800;
		letter-spacing: -0.01em;
	}

	.lead {
		margin: 6px 0 20px;
		font-size: 0.9rem;
	}

	.hint {
		font-size: 0.76rem;
		margin-top: 2px;
	}

	.hint-warn {
		color: var(--warn);
	}
</style>
