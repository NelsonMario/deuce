<script lang="ts">
	import { goto } from '$app/navigation';
	import { api, ApiError } from '$lib/api';
	import { identity } from '$lib/stores/identity';
	import { toast } from '$lib/stores/toast';

	let clubName = $state('');
	let hostName = $state('');
	let submitting = $state(false);

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		if (!clubName.trim() || !hostName.trim()) return;
		submitting = true;
		try {
			const { club, host } = await api.createClub({
				club_name: clubName.trim(),
				host_display_name: hostName.trim(),
				host_gender: 'MALE'
			});
			identity.rememberClub(club, host.token, host.player.display_name);
			toast.success(`${club.name} is live. Join code: ${club.join_code}`);
			goto(`/club/${club.id}`);
		} catch (err) {
			toast.error(err instanceof ApiError ? err.message : 'Could not create the club.');
		} finally {
			submitting = false;
		}
	}
</script>

<svelte:head>
	<title>Host a session — deuce</title>
</svelte:head>

<section class="container rise-in">
	<p class="kicker muted">step 1 of 2</p>
	<h1>Start a club</h1>
	<p class="muted lead">
		You'll get a join code straight after — share it however you're already coordinating.
	</p>

	<form onsubmit={submit} class="card stack">
		<div class="field">
			<label for="clubName">Club name</label>
			<input
				id="clubName"
				class="input"
				placeholder="Thursday Night Smash"
				bind:value={clubName}
				maxlength="80"
				autocomplete="off"
				required
			/>
		</div>

		<div class="field">
			<label for="hostName">Your name</label>
			<input
				id="hostName"
				class="input"
				placeholder="How players will see you"
				bind:value={hostName}
				maxlength="60"
				autocomplete="off"
				required
			/>
		</div>

		<button type="submit" class="btn btn-primary btn-block" disabled={submitting}>
			{submitting ? 'Creating…' : 'Create club'}
		</button>
	</form>
</section>

<style>
	.kicker {
		text-transform: uppercase;
		letter-spacing: 0.1em;
		font-size: 0.7rem;
		font-weight: 700;
		margin-bottom: 8px;
	}

	h1 {
		font-size: 1.7rem;
		font-weight: 800;
		letter-spacing: -0.01em;
	}

	.lead {
		margin: 8px 0 24px;
		font-size: 0.92rem;
	}

	form {
		margin-top: 4px;
	}
</style>
