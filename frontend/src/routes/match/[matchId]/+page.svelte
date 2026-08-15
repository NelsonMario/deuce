<script lang="ts">
	import { page } from '$app/state';
	import { api, ApiError } from '$lib/api';
	import { identity } from '$lib/stores/identity';
	import { toast } from '$lib/stores/toast';
	import { playerCache, ensurePlayers } from '$lib/stores/players';
	import { matchTeams } from '$lib/stores/matchTeams';
	import { statusLabel, formatTime } from '$lib/utils/format';
	import type { Court, Match } from '$lib/types';

	let matchId = $derived(page.params.matchId ?? '');
	let sessionId = $derived(page.url.searchParams.get('session') ?? '');
	// Reading $identity directly (not just calling identity.* methods, which
	// use the store's get() escape hatch and so aren't tracked) is what
	// makes these re-derive when the store changes — e.g. once
	// ensureCoHostChecked resolves after a co-host promotion.
	let token = $derived($identity && sessionId ? identity.tokenForSession(sessionId) : undefined);
	// Starting/finishing a match is host-only — always use the host's own token for
	// those, even if this device also joined the session as a player.
	let hostToken = $derived($identity && sessionId ? identity.hostTokenForSession(sessionId) : undefined);
	let isHost = $derived($identity && sessionId ? identity.isHostOfSession(sessionId) : false);

	let match = $state<Match | null>(null);
	let court = $state<Court | null>(null);
	let loading = $state(true);
	let loadError = $state<string | null>(null);
	let acting = $state(false);
	let scoreA = $state(0);
	let scoreB = $state(0);

	let teams = $derived($matchTeams[matchId]);

	function name(playerId: string): string {
		return $playerCache[playerId]?.display_name ?? '…';
	}

	async function load() {
		if (!token) {
			loading = false;
			return;
		}
		try {
			const [m, detail] = await Promise.all([
				api.getMatch(matchId, token),
				sessionId ? api.getSession(sessionId, token) : Promise.resolve(null)
			]);
			match = m;
			if (detail) {
				court = detail.courts.find((c) => c.id === m.court_id) ?? null;
				ensurePlayers(detail.players.map((p) => p.player_id), token);
				void identity.ensureCoHostChecked(detail.session.club_id, token);
			}
			loadError = null;
		} catch (err) {
			loadError = err instanceof ApiError ? err.message : 'Could not load this match.';
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		load();
	});

	async function start() {
		if (!hostToken) return;
		acting = true;
		try {
			match = await api.startMatch(matchId, hostToken);
			toast.success('Match started.');
		} catch (err) {
			toast.error(err instanceof ApiError ? err.message : 'Could not start the match.');
		} finally {
			acting = false;
		}
	}

	async function finish() {
		if (!hostToken) return;
		acting = true;
		try {
			match = await api.finishMatch(matchId, { score_a: scoreA, score_b: scoreB }, hostToken);
			toast.success('Match finished — ratings updated.');
		} catch (err) {
			toast.error(err instanceof ApiError ? err.message : 'Could not finish the match.');
		} finally {
			acting = false;
		}
	}
</script>

<svelte:head>
	<title>Match — deuce</title>
</svelte:head>

{#if !token}
	<section class="container rise-in">
		<div class="card">
			<p class="muted">Open this match from its session to view it.</p>
			{#if sessionId}
				<a href="/join?session={sessionId}" class="btn btn-primary" style="margin-top:16px;">Join that session</a>
			{/if}
		</div>
	</section>
{:else if loading}
	<section class="container rise-in">
		<div class="skeleton" style="height:180px;"></div>
	</section>
{:else if !match}
	<section class="container rise-in">
		<div class="card"><p class="muted">{loadError ?? 'Match not found.'}</p></div>
	</section>
{:else}
	<section class="container rise-in">
		{#if sessionId}
			<a href="/session/{sessionId}" class="back">← Back to session</a>
		{/if}

		<div class="card match-card">
			<div class="spread">
				<h1>Match</h1>
				<span class="badge dot" class:badge-live={match.status === 'PLAYING'}>{statusLabel(match.status)}</span>
			</div>
			{#if court}<p class="muted small">{court.name}</p>{/if}

			{#if teams}
				<div class="proposal">
					<div class="team">
						<span class="faint small">Team A</span>
						<p>{name(teams.a[0])} &amp; {name(teams.a[1])}</p>
					</div>
					<span class="vs">vs</span>
					<div class="team">
						<span class="faint small">Team B</span>
						<p>{name(teams.b[0])} &amp; {name(teams.b[1])}</p>
					</div>
				</div>
			{/if}

			{#if match.status === 'FINISHED'}
				<div class="final-score">
					<span class="mono score">{match.score_a}–{match.score_b}</span>
					{#if match.winner}
						<span class="badge badge-accent">Team {match.winner} won</span>
					{/if}
				</div>
				<p class="faint small">
					{formatTime(match.started_at)} – {formatTime(match.ended_at)}
				</p>
			{:else if isHost && match.status === 'CREATED'}
				<button class="btn btn-primary btn-block" onclick={start} disabled={acting}>
					{acting ? 'Starting…' : 'Start match'}
				</button>
			{:else if isHost && match.status === 'PLAYING'}
				<div class="score-form">
					<div class="field">
						<label for="scoreA">Team A score</label>
						<input id="scoreA" class="input" type="number" min="0" bind:value={scoreA} />
					</div>
					<div class="field">
						<label for="scoreB">Team B score</label>
						<input id="scoreB" class="input" type="number" min="0" bind:value={scoreB} />
					</div>
					<button class="btn btn-primary btn-block" onclick={finish} disabled={acting}>
						{acting ? 'Saving…' : 'Finish match'}
					</button>
				</div>
			{:else}
				<p class="muted small">Waiting on the host to start this match.</p>
			{/if}
		</div>
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

	.match-card h1 {
		font-size: 1.4rem;
		font-weight: 800;
	}

	.small {
		font-size: 0.8rem;
	}

	.proposal {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 16px;
		margin: 20px 0;
		padding: 16px;
		background: var(--bg-elevated-2);
		border-radius: var(--radius-sm);
		text-align: center;
	}

	.proposal .team p {
		font-weight: 700;
		margin: 4px 0;
	}

	.vs {
		color: var(--text-faint);
		font-size: 0.8rem;
	}

	.final-score {
		text-align: center;
		margin: 20px 0 8px;
	}

	.score {
		font-size: 2.4rem;
		font-weight: 800;
		display: block;
	}

	.score-form {
		margin-top: 16px;
	}

	.score-form .field {
		margin-bottom: 12px;
	}
</style>
