<script lang="ts">
	import { page } from "$app/state";
	import { goto } from "$app/navigation";
	import { api, ApiError } from "$lib/api";
	import { identity } from "$lib/stores/identity";
	import { toast } from "$lib/stores/toast";
	import { playerCache, ensurePlayers } from "$lib/stores/players";
	import PullToRefresh from "$lib/components/PullToRefresh.svelte";
	import { longpress } from "$lib/utils/gestures";
	import { reveal } from "$lib/actions/motion";
	import type { AssignmentMode, Club, Gender, Member, Session } from "$lib/types";

	let clubId = $derived(page.params.clubId ?? "");
	// Reading $identity directly (not just calling identity.* methods, which
	// use the store's get() escape hatch and so aren't tracked) is what makes
	// these re-derive when the store changes — e.g. once ensureCoHostChecked
	// resolves after a co-host promotion.
	let myToken = $derived($identity && identity.tokenForClub(clubId));
	let hostToken = $derived($identity && identity.hostTokenForClub(clubId));
	let isHost = $derived($identity && identity.isHostOfClub(clubId));
	let hostName = $derived(
		$identity.clubs[clubId]?.hostName ??
			identity.sessionForClub(clubId)?.player.display_name,
	);
	// Even without a usable token, this device may still have the club's
	// public info cached (from a previous visit) — enough to prefill the
	// rejoin form below instead of sending someone hunting for the join
	// code again.
	let rejoinCode = $derived($identity.clubs[clubId]?.joinCode);

	let club = $state<Club | null>(null);
	let loading = $state(true);
	let loadError = $state<string | null>(null);
	// True when the load failed specifically because the server rejected
	// this device's saved token (401) — as opposed to a network hiccup or a
	// genuinely deleted club — so we know it's worth offering the rejoin
	// form rather than just an error message.
	let tokenRejected = $state(false);
	// True when the server confirmed this club doesn't exist (404) — the
	// stale local entry has been pruned and the rejoin form (with code
	// field) is offered so the user can enter their current club's code,
	// which redirects to the live club.
	let clubMissing = $state(false);

	// ---- rejoin (relogin) ----
	// There's no session to go through here — this route only ever matters
	// to a host/co-host, so rejoining is a direct club-join by code, not the
	// session-join flow. If this device's device_id (see $lib/device) is
	// still the one on file server-side, joinClub hands back the SAME
	// player and role (see internal/device + club.Service.JoinClub) — a real
	// relogin, not a fresh, roleless player.
	let rejoinName = $state("");
	let rejoinGender = $state<Gender>("MALE");
	let rejoinCodeInput = $state("");
	// Set when the cached join code was rejected, so the form stops hiding
	// the code field and lets the user type the right one.
	let codeRejected = $state(false);
	let rejoining = $state(false);

	// Prefill from whatever profile this device last joined with — relogin
	// becomes one tap instead of retyping name/gender.
	$effect(() => {
		const p = $identity.lastProfile;
		if (p && !rejoinName) {
			rejoinName = p.displayName;
			rejoinGender = p.gender;
		}
	});

	async function doJoin(code: string, displayName: string, gender: Gender, silent: boolean) {
		rejoining = true;
		try {
			const { club: joined, you } = await api.joinClub(clubId, {
				join_code: code.trim().toUpperCase(),
				display_name: displayName.trim(),
				gender,
			});
			// Refresh the cached name/code from the server's answer — storage
			// must never outlive the truth.
			identity.cacheClubInfo(joined);
			identity.rememberClubMembership(clubId, you.token, you.player);
			await identity.ensureCoHostChecked(clubId, you.token);
			toast.success(`Welcome back, ${you.player.display_name}.`);
			return true;
		} catch (err) {
			codeRejected = true;
			// The club in this URL may no longer exist server-side (fresh
			// database, deleted club). The join CODE knows where home is now:
			// resolve it and follow it to the live club instead of failing on
			// a stale ID forever.
			if (err instanceof ApiError && err.status === 404) {
				identity.forgetClub(clubId);
				try {
					const resolved = await api.resolveClub({
						join_code: code.trim().toUpperCase(),
					});
					toast.info(`${resolved.club.name} is your club now.`);
					await goto(`/club/${resolved.club.id}`);
					return false;
				} catch {
					if (!silent) {
						toast.error(
							"That club doesn't exist anymore — enter your current club's code.",
						);
					}
					return false;
				}
			}
			if (!silent) {
				toast.error(
					err instanceof ApiError ? err.message : "Couldn't sign back in.",
				);
			}
			return false;
		} finally {
			rejoining = false;
		}
	}

	async function rejoin(e: SubmitEvent) {
		e.preventDefault();
		const code = (rejoinCode && !codeRejected ? rejoinCode : rejoinCodeInput);
		if (!code.trim() || !rejoinName.trim()) return;
		await doJoin(code, rejoinName, rejoinGender, false);
	}

	// Zero-tap relogin: the moment this page loads signed-out with both a
	// cached club code AND a remembered profile, just try it — the server's
	// device_id recognition does the rest. Only a failure surfaces the form.
	let autoRejoinAttempted = $state(false);

	$effect(() => {
		if (loading || myToken || autoRejoinAttempted || rejoining) return;
		autoRejoinAttempted = true;
		const p = $identity.lastProfile;
		if (rejoinCode && !codeRejected && p?.displayName) {
			void doJoin(rejoinCode, p.displayName, p.gender, true).then((ok) => {
				if (!ok) toast.info("Couldn't sign you in automatically — enter your details below.");
			});
		}
	});

	async function load() {
		if (!myToken) {
			loading = false;
			return;
		}
		try {
			club = await api.getClub(clubId, myToken);
			// Cache join_code/name for THIS device too — not just the device that
			// created the club — so e.g. a promoted co-host's session page can
			// resolve identity.club(clubId) for the invite link.
			identity.cacheClubInfo(club);
			await identity.ensureCoHostChecked(clubId, myToken);
			loadError = null;
			tokenRejected = false;
			clubMissing = false;
		} catch (err) {
			loadError =
				err instanceof ApiError
					? err.message
					: "Could not load this club.";
			tokenRejected = err instanceof ApiError && err.status === 401;
			// The server confirms this club no longer exists — stop trusting
			// the stale local entry (links, cached code) and offer the
			// code-based way back in.
			if (err instanceof ApiError && err.status === 404) {
				clubMissing = true;
				identity.forgetClub(clubId);
				loadError = "This club no longer exists.";
			}
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		load();
	});

	let members = $state<Member[]>([]);
	let membersLoaded = $state(false);
	// Sessions belonging to this club, fetched from the server — not each
	// device's own local history — so every co-host sees every session,
	// regardless of which device created it.
	let sessions = $state<Session[]>([]);

	async function loadMembers() {
		if (!hostToken) return;
		try {
			const [membersRes, sessionsRes] = await Promise.all([
				api.listClubMembers(clubId, hostToken),
				api.listClubSessions(clubId, hostToken),
			]);
			members = membersRes.members;
			sessions = sessionsRes.sessions;
			membersLoaded = true;
			ensurePlayers(
				members.map((m) => m.player_id),
				hostToken,
			);
		} catch (err) {
			toast.error(
				err instanceof ApiError
					? err.message
					: "Could not load members.",
			);
		}
	}

	$effect(() => {
		if (isHost && hostToken) void loadMembers();
	});

	let promotingId = $state<string | null>(null);

	async function promote(playerId: string) {
		if (!hostToken) return;
		promotingId = playerId;
		try {
			await api.promoteMember(clubId, playerId, hostToken);
			toast.success("Co-host promoted.");
			await loadMembers();
		} catch (err) {
			toast.error(
				err instanceof ApiError
					? err.message
					: "Could not promote that player.",
			);
		} finally {
			promotingId = null;
		}
	}

	function name(playerId: string): string {
		return $playerCache[playerId]?.display_name ?? "…";
	}

	let sessionName = $state("");
	let mode = $state<AssignmentMode>("AUTOMATIC");
	let creating = $state(false);
	let copied = $state(false);

	async function copyCode() {
		if (!club) return;
		try {
			await navigator.clipboard.writeText(club.join_code);
			copied = true;
			setTimeout(() => (copied = false), 1600);
		} catch {
			toast.info(`Join code: ${club.join_code}`);
		}
	}

	async function refresh() {
		await load();
		if (isHost && hostToken) await loadMembers();
	}

	async function createSession(e: SubmitEvent) {
		e.preventDefault();
		if (!club || !hostToken) return;
		creating = true;
		try {
			const session = await api.createSession(hostToken, {
				club_id: club.id,
				name: sessionName.trim() || undefined,
				assignment_mode: mode,
			});
			identity.setSessionClub(session.id, club.id);
			toast.success("Session created.");
			goto(`/session/${session.id}`);
		} catch (err) {
			toast.error(
				err instanceof ApiError
					? err.message
					: "Could not create the session.",
			);
		} finally {
			creating = false;
		}
	}
</script>

<svelte:head>
	<title>{club?.name ?? "Club"} — deuce</title>
</svelte:head>

{#snippet rejoinForm()}
	<form onsubmit={rejoin} class="stack" style="margin-top:16px;">
		{#if !rejoinCode || codeRejected}
			<div class="field">
				<label for="rejoinCode">Join code</label>
				<input
					id="rejoinCode"
					class="input mono"
					placeholder="e.g. 4K9P2Q"
					bind:value={rejoinCodeInput}
					autocomplete="off"
					style="letter-spacing:0.08em; text-transform:uppercase;"
					required
				/>
			</div>
		{/if}
		<div class="field">
			<label for="rejoinName">Your name</label>
			<input
				id="rejoinName"
				class="input"
				placeholder="The name you joined with"
				bind:value={rejoinName}
				maxlength="60"
				autocomplete="off"
				required
			/>
		</div>
		<div class="field">
			<label for="rejoinGender">Gender</label>
			<div
				class="segmented"
				role="radiogroup"
				aria-label="Gender"
				id="rejoinGender"
			>
				<button
					type="button"
					class:active={rejoinGender === "MALE"}
					onclick={() => (rejoinGender = "MALE")}
				>
					Male
				</button>
				<button
					type="button"
					class:active={rejoinGender === "FEMALE"}
					onclick={() => (rejoinGender = "FEMALE")}
				>
					Female
				</button>
			</div>
		</div>
		<button
			type="submit"
			class="btn btn-primary btn-block"
			disabled={rejoining}
		>
			{rejoining ? "Signing in…" : "Sign back in"}
		</button>
	</form>
{/snippet}

<PullToRefresh onrefresh={refresh}>
	{#if loading}
		<section class="container rise-in">
			<div class="skeleton" style="height:160px;"></div>
		</section>
	{:else if !myToken}
		<section class="container rise-in">
			<div class="card">
				<p class="muted">
					You're signed out on this device. If this device was
					recognized before, rejoining below picks up the same player
					and role — no need to start over.
				</p>
				{@render rejoinForm()}
				<div class="row" style="margin-top:16px;">
					<a href="/host" class="btn btn-ghost">Start a new club</a>
					<a href="/join" class="btn btn-ghost"
						>Join a session with a code</a
					>
				</div>
			</div>
		</section>
	{:else if !club}
		<section class="container rise-in">
			<div class="card">
				<p class="muted">{loadError ?? "Club not found."}</p>
				{#if clubMissing}
					<p class="muted small" style="margin-top:8px;">
						This link points at a club that's gone. Enter your
						current club's code below and we'll take you to the
						right place — or start a new club.
					</p>
					{@render rejoinForm()}
					<div class="row" style="margin-top:16px;">
						<a href="/host" class="btn btn-ghost">Start a new club</a>
						<a href="/join" class="btn btn-ghost">Join by code</a>
					</div>
				{:else if tokenRejected}
					<p class="muted small" style="margin-top:8px;">
						This device's saved login stopped working. Rejoining
						below picks up the same player and role.
					</p>
					{@render rejoinForm()}
				{/if}
			</div>
		</section>
	{:else if !isHost}
		<section class="container rise-in">
			<h1>{club.name}</h1>
			<div class="card">
				<p class="muted">
					You're a member of this club, but not a host — only hosts
					create sessions and manage members. Ask a host to promote
					you if you need that.
				</p>
			</div>
		</section>
	{:else}
		<section class="container rise-in">
			<p class="kicker muted">step 2 of 2</p>
			<h1>{club.name}</h1>
			<p class="muted lead">Hosting{hostName ? ` as ${hostName}` : ""}</p>

			<div class="card join-card card-pop" use:reveal={{ delay: 120 }}>
				<span class="muted small">Join code · hold to copy</span>
				<button
					type="button"
					class="code mono code-press"
					class:copied
					use:longpress={copyCode}
					aria-label="Hold to copy join code"
				>
					{club.join_code}
				</button>
				<p class="faint small">
					Players enter this code (and the session link) to join.
				</p>
			</div>

			<h2 class="section-title">Co-hosts &amp; members</h2>
			<div class="stack">
				{#each members as m (m.player_id)}
					<div class="card card-tight row spread">
						<span>{name(m.player_id)}</span>
						<div class="row">
							{#if m.role === "HOST"}
								<span class="badge badge-accent">Co-host</span>
							{:else}
								<button
									class="btn btn-ghost btn-sm"
									onclick={() => promote(m.player_id)}
									disabled={promotingId === m.player_id}
								>
									{promotingId === m.player_id
										? "Promoting…"
										: "Make co-host"}
								</button>
							{/if}
						</div>
					</div>
				{/each}
				{#if membersLoaded && members.length === 0}
					<p class="muted small">No members yet.</p>
				{/if}
			</div>

			{#if sessions.length}
				<h2 class="section-title">Your sessions</h2>
				<div class="stack">
					{#each sessions as s (s.id)}
						<a
							href="/session/{s.id}"
							class="card card-tight session-link row spread"
						>
							<span class="mono small"
								>{s.name || s.id.slice(0, 8)}</span
							>
							<span class="muted small">Open →</span>
						</a>
					{/each}
				</div>
				<hr class="divider" />
				<h2 class="section-title">Start another session</h2>
			{:else}
				<h2 class="section-title">Start today's session</h2>
			{/if}

			<form onsubmit={createSession} class="card stack">
				<div class="field">
					<label for="sessionName">Session name (optional)</label>
					<input
						id="sessionName"
						class="input"
						placeholder="e.g. Week 12"
						bind:value={sessionName}
						maxlength="100"
						autocomplete="off"
					/>
				</div>

				<div class="field">
					<label for="mode">Match assignment</label>
					<div
						class="segmented"
						role="radiogroup"
						aria-label="Assignment mode"
					>
						<button
							type="button"
							class:active={mode === "AUTOMATIC"}
							onclick={() => (mode = "AUTOMATIC")}
						>
							Automatic
						</button>
						<button
							type="button"
							class:active={mode === "MANUAL"}
							onclick={() => (mode = "MANUAL")}
						>
							Manual
						</button>
					</div>
					<p class="faint small">
						{mode === "AUTOMATIC"
							? "The engine picks the next four players by rating and wait time."
							: "You'll pick the four, we'll suggest fair teams."}
					</p>
				</div>

				<button
					type="submit"
					class="btn btn-primary btn-block"
					disabled={creating}
				>
					{creating ? "Creating…" : "Create session"}
				</button>
			</form>
		</section>
	{/if}
</PullToRefresh>

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
		margin: 6px 0 20px;
		font-size: 0.9rem;
	}

	.join-card {
		margin-bottom: 28px;
	}

	.small {
		font-size: 0.78rem;
	}

	.code {
		display: block;
		width: 100%;
		background: none;
		border: none;
		padding: 0;
		text-align: left;
		font-size: 1.7rem;
		font-weight: 800;
		letter-spacing: 0.08em;
		color: var(--accent);
		cursor: pointer;
		transition: opacity 0.15s ease, letter-spacing 0.3s cubic-bezier(0.16, 1, 0.3, 1);
	}

	.code:hover {
		letter-spacing: 0.16em;
	}

	.code-press {
		animation: code-glow 2.6s ease-in-out infinite;
	}

	@keyframes code-glow {
		0%,
		100% {
			text-shadow: 0 0 0 transparent;
		}
		50% {
			text-shadow:
				0 0 18px color-mix(in srgb, var(--accent) 45%, transparent),
				2px 2px 0 var(--pop-pink);
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.code-press {
			animation: none;
		}
	}

	.code.copied {
		opacity: 0.5;
	}

	.faint.small {
		margin-top: 8px;
	}

	.section-title {
		font-size: 0.95rem;
		font-weight: 700;
		margin: 20px 0 10px;
	}

	.session-link {
		transition: border-color 0.15s ease;
	}

	.session-link:hover {
		border-color: var(--text-dim);
	}
</style>
