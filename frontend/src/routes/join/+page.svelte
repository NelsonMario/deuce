<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { api, ApiError } from '$lib/api';
	import { identity } from '$lib/stores/identity';
	import { toast } from '$lib/stores/toast';
	import type { Gender, PlayerAuth } from '$lib/types';

	const UUID_RE = /[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/i;

	// Joining by club code is the default path; full invite links
	// (?session=…&code=…) flip this to the session form automatically.
	let mode = $state<'club' | 'session'>(
		page.url.searchParams.get('session') ? 'session' : 'club'
	);
	let sessionLink = $state(page.url.searchParams.get('session') ?? '');
	let joinCode = $state(page.url.searchParams.get('code') ?? '');
	let displayName = $state('');
	let gender = $state<Gender>('MALE');
	let submitting = $state(false);

	let sessionId = $derived(sessionLink.match(UUID_RE)?.[0] ?? '');
	let canSubmit = $derived(mode === 'club' ? !!joinCode.trim() : !!sessionId);

	// Returning device? Prefill the profile it last joined with so rejoining
	// is a single tap on the submit button.
	$effect(() => {
		const p = $identity.lastProfile;
		if (p && !displayName) {
			displayName = p.displayName;
			gender = p.gender;
		}
	});

	async function welcomeToast(you: PlayerAuth) {
		if (!you.returning) {
			toast.success(`You're in, ${you.player.display_name}.`);
			return;
		}
		const rating = await api.getPlayerRating(you.player.id, you.token).catch(() => null);
		toast.success(
			rating
				? `Welcome back, ${you.player.display_name} — rating ${Math.round(rating.rating)}.`
				: `Welcome back, ${you.player.display_name}.`
		);
	}

	async function joinActiveSession(sessionIdToJoin: string, code: string): Promise<boolean> {
		try {
			const { session, session_player, you } = await api.joinSession(sessionIdToJoin, {
				join_code: code,
				display_name: displayName.trim(),
				gender
			});
			identity.rememberSession(session.id, {
				clubId: session.club_id,
				token: you.token,
				sessionPlayerId: session_player.id,
				player: you.player
			});
			void welcomeToast(you);
			goto(`/session/${session.id}`);
			return true;
		} catch {
			// e.g. the session was just ended or deleted — falling back to the
			// club page beats an error screen.
			return false;
		}
	}

	async function joinByClubCode(code: string) {
		const resolved = await api.resolveClub({ join_code: code });
		const clubId = resolved.club.id;

		// joinClub recognizes returning devices server-side (device_id), so
		// this doubles as the club-code relogin path — no stored session
		// state involved, so nothing can go stale here.
		const { you } = await api.joinClub(clubId, {
			join_code: code,
			display_name: displayName.trim(),
			gender
		});
		identity.cacheClubInfo(resolved.club);
		identity.rememberClubMembership(clubId, you.token, you.player);
		void welcomeToast(you);

		const active = resolved.active_sessions?.[0];
		if (active && (await joinActiveSession(active.id, code))) return;
		if (active) {
			toast.info("Couldn't seat you in the live session — here's your club instead.");
		}
		goto(`/club/${clubId}`);
	}

	async function joinSessionById(id: string) {
		const { session, session_player, you } = await api.joinSession(id, {
			join_code: joinCode.trim().toUpperCase(),
			display_name: displayName.trim(),
			gender
		});
		identity.rememberSession(session.id, {
			clubId: session.club_id,
			token: you.token,
			sessionPlayerId: session_player.id,
			player: you.player
		});
		void welcomeToast(you);
		goto(`/session/${session.id}`);
	}

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		if (!displayName.trim() || !canSubmit || submitting) return;
		submitting = true;
		try {
			if (mode === 'session') {
				await joinSessionById(sessionId);
			} else {
				await joinByClubCode(joinCode.trim().toUpperCase());
			}
		} catch (err) {
			toast.error(
				err instanceof ApiError
					? err.message
					: mode === 'session'
						? "Couldn't join that session."
						: "Couldn't find a club with that code."
			);
		} finally {
			submitting = false;
		}
	}
</script>

<svelte:head>
	<title>Join — deuce</title>
</svelte:head>

<section class="container rise-in">
	<h1>Join your club</h1>
	<p class="muted lead">Enter the club code your host shared — we'll seat you in the live session if one is running.</p>

	<div class="segmented mode-toggle" role="tablist" aria-label="Join method">
		<button type="button" role="tab" aria-selected={mode === 'club'} class:active={mode === 'club'} onclick={() => (mode = 'club')}>
			Club code
		</button>
		<button type="button" role="tab" aria-selected={mode === 'session'} class:active={mode === 'session'} onclick={() => (mode = 'session')}>
			Session link
		</button>
	</div>

	<form onsubmit={submit} class="card stack">
		{#if mode === 'session'}
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
		{/if}

		<div class="field">
			<label for="code">{mode === 'club' ? 'Club code' : 'Join code'}</label>
			<input
				id="code"
				class="input mono"
				placeholder="e.g. 4K9P2Q"
				bind:value={joinCode}
				autocomplete="off"
				style="letter-spacing:0.08em; text-transform:uppercase;"
				required
			/>
			{#if mode === 'club'}
				<span class="hint faint">The permanent code for the club — same one every week.</span>
			{/if}
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

		<button type="submit" class="btn btn-primary btn-block" disabled={submitting || !canSubmit}>
			{submitting ? 'Joining…' : mode === 'club' ? 'Join club' : 'Join session'}
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

	.mode-toggle {
		margin-bottom: 16px;
		max-width: 320px;
	}

	.hint {
		font-size: 0.76rem;
		margin-top: 2px;
	}

	.hint-warn {
		color: var(--warn);
	}

	/* the code field physically opens its letter-spacing while you type —
	   small, but it makes the one-field-that-matters feel deliberate */
	#code {
		transition:
			border-color 0.15s ease,
			box-shadow 0.15s ease,
			letter-spacing 0.3s cubic-bezier(0.16, 1, 0.3, 1);
	}

	#code:focus {
		letter-spacing: 0.22em;
	}
</style>
