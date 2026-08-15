<script lang="ts">
	import { page } from '$app/state';
	import { api, ApiError } from '$lib/api';
	import { identity } from '$lib/stores/identity';
	import { statusLabel, initials } from '$lib/utils/format';
	import type { MatchSummary, Player, Rating } from '$lib/types';

	let playerId = $derived(page.params.playerId ?? '');
	let sessionParam = $derived(page.url.searchParams.get('session'));
	let token = $derived(
		(sessionParam && identity.tokenForSession(sessionParam)) ??
			identity.sessionForPlayer(playerId)?.token ??
			identity.anyToken()
	);
	let isMe = $derived(identity.sessionForPlayer(playerId) !== undefined);

	let player = $state<Player | null>(null);
	let rating = $state<Rating | null>(null);
	let matches = $state<MatchSummary[]>([]);
	let loading = $state(true);
	let loadError = $state<string | null>(null);

	async function load() {
		if (!token) {
			loading = false;
			return;
		}
		try {
			const [p, r, m] = await Promise.all([
				api.getPlayer(playerId, token),
				api.getPlayerRating(playerId, token),
				api.listPlayerMatches(playerId, token, { limit: 20 })
			]);
			player = p;
			rating = r;
			matches = m.matches;
			loadError = null;
		} catch (err) {
			loadError = err instanceof ApiError ? err.message : 'Could not load this player.';
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		load();
	});

	let played = $derived(matches.filter((m) => m.status === 'FINISHED').length);
</script>

<svelte:head>
	<title>{player?.display_name ?? 'Player'} — deuce</title>
</svelte:head>

{#if !token}
	<section class="container rise-in">
		<div class="card"><p class="muted">Join a session first to view player profiles.</p></div>
	</section>
{:else if loading}
	<section class="container rise-in">
		<div class="skeleton" style="height:160px;"></div>
	</section>
{:else if !player}
	<section class="container rise-in">
		<div class="card"><p class="muted">{loadError ?? 'Player not found.'}</p></div>
	</section>
{:else}
	<section class="container rise-in">
		{#if sessionParam}
			<a href="/session/{sessionParam}" class="back">← Back to session</a>
		{/if}

		<div class="card profile-card">
			<div class="row">
				<div class="avatar">{initials(player.display_name)}</div>
				<div>
					<h1>{player.display_name} {#if isMe}<span class="badge badge-accent">You</span>{/if}</h1>
					<p class="muted small">{played} matches played</p>
				</div>
			</div>

			{#if rating}
				<div class="rating-block">
					<span class="rating mono">{Math.round(rating.rating)}</span>
					<span class="faint small">rating</span>
				</div>
			{/if}
		</div>

		<h2 class="section-title">Recent matches</h2>
		{#if matches.length === 0}
			<p class="muted small">No matches yet.</p>
		{:else}
			<div class="stack">
				{#each matches as m}
					<div class="card card-tight match-row spread">
						<span>Match</span>
						<span class="row">
							{#if m.status === 'FINISHED'}
								<span class="mono">{m.score_a}–{m.score_b}</span>
							{/if}
							<span class="badge dot">{statusLabel(m.status)}</span>
						</span>
					</div>
				{/each}
			</div>
		{/if}
	</section>
{/if}

<style>
	.back {
		display: inline-block;
		margin-bottom: 16px;
		font-size: 0.85rem;
		color: var(--text-dim);
	}

	.back:hover {
		color: var(--accent);
	}

	.profile-card {
		display: flex;
		align-items: center;
		justify-content: space-between;
		flex-wrap: wrap;
		gap: 16px;
	}

	.avatar {
		width: 48px;
		height: 48px;
		border-radius: 50%;
		background: var(--bg-elevated-2);
		color: var(--accent);
		display: flex;
		align-items: center;
		justify-content: center;
		font-weight: 800;
		font-size: 0.95rem;
	}

	h1 {
		font-size: 1.3rem;
		font-weight: 800;
	}

	.small {
		font-size: 0.8rem;
	}

	.rating-block {
		text-align: center;
	}

	.rating {
		display: block;
		font-size: 1.8rem;
		font-weight: 800;
		color: var(--accent);
	}

	.section-title {
		font-size: 0.95rem;
		font-weight: 700;
		margin: 24px 0 10px;
	}
</style>
