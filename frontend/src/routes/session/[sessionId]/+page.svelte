<script lang="ts">
	import { page } from "$app/state";
	import { afterNavigate } from "$app/navigation";
	import { api, ApiError } from "$lib/api";
	import { identity } from "$lib/stores/identity";
	import { toast } from "$lib/stores/toast";
	import {
		playerCache,
		ensurePlayers,
		ratingCache,
		ensureRatings,
		updateCachedRating,
	} from "$lib/stores/players";
	import { matchTeams, rememberMatchTeams } from "$lib/stores/matchTeams";
	import { statusLabel, mmss } from "$lib/utils/format";
	import {
		getRank,
		getRankBadgeClass,
		rankToBaseRating,
		RANK_TIERS,
	} from "$lib/utils/rank";
	import PullToRefresh from "$lib/components/PullToRefresh.svelte";
	import { longpress } from "$lib/utils/gestures";
	import type {
		AssignmentMode,
		Court,
		Gender,
		Match,
		MatchFormat,
		Session,
		SessionPlayer,
	} from "$lib/types";

	let sessionId = $derived(page.params.sessionId ?? "");
	// Reading $identity directly (not just calling identity.* methods, which
	// use the store's get() escape hatch and so aren't tracked) is what
	// makes these re-derive when the store changes — e.g. once
	// ensureCoHostChecked resolves after a co-host promotion.
	let token = $derived($identity && identity.tokenForSession(sessionId));
	// Host-only actions must always authenticate as the host, even if this device also
	// joined the session as a player (tokenForSession prefers the player identity for reads).
	let hostToken = $derived(
		$identity && identity.hostTokenForSession(sessionId),
	);
	let isHost = $derived(
		$identity.clubBySession[sessionId] &&
			identity.isHostOfSession(sessionId),
	);
	let mySessionPlayerId = $derived(
		$identity.sessions[sessionId]?.sessionPlayerId,
	);

	let session = $state<Session | null>(null);
	let players = $state<SessionPlayer[]>([]);
	let courts = $state<Court[]>([]);
	let matches = $state<Match[]>([]);
	let loading = $state(true);
	let loadError = $state<string | null>(null);
	let polledAt = $state(Date.now());
	let tick = $state(0);

	// Collapsible section state
	let courtsCollapsed = $state(false);
	let playersCollapsed = $state(false);
	let currentMatchesCollapsed = $state(false);
	let historicalMatchesCollapsed = $state(false);
	let showRankMode = $state<"both" | "rank" | "rating">("both");

	// Host Edit Rating state
	let editRatingPlayerId = $state<string | null>(null);
	let editRatingVal = $state<number>(1000);
	let editRankVal = $state<string>("C");
	let savingRating = $state(false);

	function openEditRating(playerId: string) {
		if (!isHost) return;
		editRatingPlayerId = playerId;
		const r = rating(playerId) ?? 1000;
		editRatingVal = Math.round(r);
		editRankVal = getRank(r);
	}

	function onRankSelectChange(newRank: string) {
		editRankVal = newRank;
		editRatingVal = rankToBaseRating(newRank);
	}

	async function savePlayerRating() {
		if (!hostToken || !editRatingPlayerId) return;
		savingRating = true;
		try {
			await api.updatePlayerRating(
				editRatingPlayerId,
				editRatingVal,
				hostToken,
			);
			updateCachedRating(editRatingPlayerId, editRatingVal);
			toast.success(`Updated rating for ${name(editRatingPlayerId)}`);
			editRatingPlayerId = null;
		} catch (err) {
			toast.error(
				err instanceof ApiError
					? err.message
					: "Failed to update rating.",
			);
		} finally {
			savingRating = false;
		}
	}

	// courtId -> playerIds currently believed to occupy it (inferred client-side; the
	// API doesn't expose match rosters, see notes on generateAuto/confirmManual below).
	let courtOccupants = $state<Record<string, string[]>>({});

	let myPlayer = $derived(
		players.find(
			(p) => p.id === mySessionPlayerId || p.player_id === myPlayerId,
		),
	);
	let waiting = $derived(
		players
			.filter((p) => p.status === "WAITING")
			.sort((a, b) => b.waiting_seconds - a.waiting_seconds),
	);
	let playing = $derived(players.filter((p) => p.status === "PLAYING"));
	let onBreak = $derived(players.filter((p) => p.status === "BREAK"));
	let ended = $derived(players.filter((p) => p.status === "ENDED"));
	let availableCourts = $derived(
		courts.filter((c) => c.status === "AVAILABLE"),
	);
	let currentMatches = $derived(
		matches.filter((m) => m.status === "PLAYING"),
	);
	let historicalMatches = $derived(
		matches.filter((m) => m.status === "FINISHED"),
	);
	let latestMatches = $derived(matches.slice(0, 3));

	function courtActiveMatch(courtId: string): Match | undefined {
		return matches.find(
			(m) => m.court_id === courtId && m.status === "PLAYING",
		);
	}
	let myPlayerId = $derived(
		$identity &&
			($identity.sessions[sessionId]?.player.id ??
				(session
					? identity.sessionForClub(session.club_id)?.player.id
					: undefined) ??
				(session
					? identity.club(session.club_id)?.hostPlayerId
					: undefined)),
	);

	function name(playerId: string): string {
		return $playerCache[playerId]?.display_name ?? "…";
	}

	function rating(playerId: string): number | undefined {
		return $ratingCache[playerId];
	}

	function isGuest(playerId: string): boolean {
		return $playerCache[playerId]?.is_guest ?? false;
	}

	function liveWaitingSeconds(p: SessionPlayer, _: number): number {
		if (p.status !== "WAITING") return p.waiting_seconds;
		return (
			p.waiting_seconds +
			Math.max(0, Math.floor((Date.now() - polledAt) / 1000))
		);
	}

	async function applyDetail(detail: {
		session: Session;
		players: SessionPlayer[];
		courts: Court[];
	}) {
		const prevPlaying = new Set(
			players
				.filter((p) => p.status === "PLAYING")
				.map((p) => p.player_id),
		);
		session = detail.session;
		players = detail.players;
		courts = detail.courts;
		polledAt = Date.now();
		identity.setSessionClub(sessionId, detail.session.club_id);
		if (token) {
			ensurePlayers(
				detail.players.map((p) => p.player_id),
				token,
			);
			void identity.ensureCoHostChecked(detail.session.club_id, token);
			// A promoted co-host's device may never have visited the club page
			// (where this is normally cached), so identity.club(...) — and thus
			// the invite link below — would otherwise stay empty forever. Any
			// authenticated device can read a club's public info, so just fetch
			// it once if we don't already have it.
			if (!identity.club(detail.session.club_id)) {
				api.getClub(detail.session.club_id, token)
					.then((c) => identity.cacheClubInfo(c))
					.catch(() => {});
			}
		}

		// clear occupants for courts that freed up
		for (const c of detail.courts) {
			if (c.status === "AVAILABLE" && courtOccupants[c.id]) {
				const next = { ...courtOccupants };
				delete next[c.id];
				courtOccupants = next;
			}
		}
	}

	// No background polling: fetched once on load, after every action
	// that changes something, when this page regains visibility (tab/app
	// returns to foreground), and when returning to this route via navigation.
	// To manually refresh, pull down from the top of the page.
	async function poll() {
		if (!token) return;
		try {
			const [detail, matchList] = await Promise.all([
				api.getSession(sessionId, token),
				api.listSessionMatches(sessionId, token),
			]);
			await applyDetail(detail);
			matches = matchList.matches;

			// Populate court occupants and matchTeams using player rosters
			const nextOccupants = { ...courtOccupants };
			for (const m of matches) {
				if (m.players && m.players.length === 4) {
					rememberMatchTeams(
						m.id,
						[m.players[0], m.players[1]],
						[m.players[2], m.players[3]],
					);
					nextOccupants[m.court_id] = m.players;
				}
			}
			courtOccupants = nextOccupants;

			loadError = null;
		} catch (err) {
			loadError =
				err instanceof ApiError
					? err.message
					: "Lost connection to the server.";
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		if (!token) {
			loading = false;
			return;
		}
		poll();
	});

	$effect(() => {
		const timer = setInterval(() => (tick += 1), 1000);
		return () => clearInterval(timer);
	});

	$effect(() => {
		function onVisibilityChange() {
			if (!document.hidden && token) {
				void poll();
			}
		}
		document.addEventListener("visibilitychange", onVisibilityChange);
		return () =>
			document.removeEventListener(
				"visibilitychange",
				onVisibilityChange,
			);
	});

	afterNavigate(() => {
		if (token) void poll();
	});

	// ---- host: session lifecycle ----
	let busy = $state(false);

	async function startSession() {
		if (!hostToken) return;
		busy = true;
		try {
			session = await api.startSession(sessionId, hostToken);
			toast.success("Session started. Get moving!");
		} catch (err) {
			toast.error(
				err instanceof ApiError
					? err.message
					: "Could not start the session.",
			);
		} finally {
			busy = false;
		}
	}

	async function endSession() {
		if (!hostToken) return;
		if (!confirm("End this session for everyone?")) return;
		busy = true;
		try {
			session = await api.endSession(sessionId, hostToken);
			toast.info("Session ended.");
		} catch (err) {
			toast.error(
				err instanceof ApiError
					? err.message
					: "Could not end the session.",
			);
		} finally {
			busy = false;
		}
	}

	// ---- host: matchmaking settings (mid-session) ----
	let changingMode = $state(false);
	let togglingAutoFill = $state(false);

	async function changeMode(mode: AssignmentMode) {
		if (!hostToken || !session || session.assignment_mode === mode) return;
		changingMode = true;
		try {
			session = await api.setAssignmentMode(sessionId, mode, hostToken);
			toast.success(
				mode === "AUTOMATIC"
					? "Automatic assignment on."
					: "Manual assignment on.",
			);
		} catch (err) {
			toast.error(
				err instanceof ApiError
					? err.message
					: "Could not change assignment mode.",
			);
		} finally {
			changingMode = false;
		}
	}

	async function setAutoFill(enabled: boolean) {
		if (!hostToken || !session || session.auto_fill_enabled === enabled)
			return;
		togglingAutoFill = true;
		try {
			session = await api.setAutoFillEnabled(
				sessionId,
				enabled,
				hostToken,
			);
			toast.success(enabled ? "Auto-fill on." : "Auto-fill off.");
		} catch (err) {
			toast.error(
				err instanceof ApiError
					? err.message
					: "Could not change auto-fill.",
			);
		} finally {
			togglingAutoFill = false;
		}
	}

	// ---- host: courts ----
	let newCourtName = $state("");
	let addingCourt = $state(false);

	async function addCourt(e: SubmitEvent) {
		e.preventDefault();
		if (!hostToken || !newCourtName.trim()) return;
		addingCourt = true;
		try {
			await api.createCourt(sessionId, newCourtName.trim(), hostToken);
			newCourtName = "";
			toast.success("Court added.");
			poll();
		} catch (err) {
			toast.error(
				err instanceof ApiError
					? err.message
					: "Could not add the court.",
			);
		} finally {
			addingCourt = false;
		}
	}

	// ---- host: guest registration ----
	let guestNames = $state("");
	let guestDraft = $state<{ name: string; gender: Gender }[]>([]);
	let addingGuests = $state(false);

	function parseGuestNames() {
		const parts = guestNames
			.split(/[\n,]+/)
			.map((s) => s.trim())
			.filter(Boolean);
		const existing = new Set(guestDraft.map((g) => g.name.toLowerCase()));
		for (const p of parts) {
			if (existing.has(p.toLowerCase())) continue;
			existing.add(p.toLowerCase());
			guestDraft = [...guestDraft, { name: p, gender: "MALE" }];
		}
		guestNames = "";
	}

	function setGuestGender(i: number, gender: Gender) {
		guestDraft = guestDraft.map((g, idx) =>
			idx === i ? { ...g, gender } : g,
		);
	}

	function removeGuest(i: number) {
		guestDraft = guestDraft.filter((_, idx) => idx !== i);
	}

	async function addGuests() {
		if (!hostToken || guestDraft.length === 0) return;
		addingGuests = true;
		try {
			await api.registerGuests(
				sessionId,
				{
					guests: guestDraft.map((g) => ({
						display_name: g.name,
						gender: g.gender,
					})),
				},
				hostToken,
			);
			const count = guestDraft.length;
			guestDraft = [];
			toast.success(`Added ${count} guest${count > 1 ? "s" : ""}.`);
			poll();
		} catch (err) {
			toast.error(
				err instanceof ApiError ? err.message : "Could not add guests.",
			);
		} finally {
			addingGuests = false;
		}
	}

	// ---- host: automatic match generation ----
	let autoFormat = $state<MatchFormat>("MIXED_DOUBLES");
	let autoCourtId = $state("");
	let generating = $state(false);

	$effect(() => {
		if (!autoCourtId && availableCourts.length)
			autoCourtId = availableCourts[0].id;
	});

	async function generateAuto() {
		if (!hostToken || !autoCourtId) return;
		if (session && !session.auto_fill_enabled) {
			generating = true;
			try {
				const p = await api.previewAutoMatch(
					sessionId,
					{ format: autoFormat },
					hostToken,
				);
				proposal = {
					a: p.team_a,
					b: p.team_b,
					ratingA: p.team_a_rating,
					ratingB: p.team_b_rating,
				};
				manualCourtId = autoCourtId;
				manualFormat = autoFormat;
				openHud("match");
				toast.info(
					"Match preview generated! You can replace or swap players before confirming.",
				);
			} catch (err) {
				toast.error(
					err instanceof ApiError
						? err.message
						: "Could not generate match preview.",
				);
			} finally {
				generating = false;
			}
			return;
		}

		const before = new Set(waiting.map((p) => p.player_id));
		generating = true;
		try {
			await api.generateMatch(
				sessionId,
				{ court_id: autoCourtId, format: autoFormat },
				hostToken,
			);
			const detail = await api.getSession(sessionId, hostToken);
			const nowPlaying = detail.players.filter(
				(p) => p.status === "PLAYING" && before.has(p.player_id),
			);
			if (nowPlaying.length) {
				courtOccupants = {
					...courtOccupants,
					[autoCourtId]: nowPlaying.map((p) => p.player_id),
				};
			}
			await applyDetail(detail);
			toast.success("Match assigned. On court!");
		} catch (err) {
			toast.error(
				err instanceof ApiError
					? err.message
					: "Could not generate a match.",
			);
		} finally {
			generating = false;
		}
	}

	// ---- host: manual match builder ----
	let manualFormat = $state<MatchFormat>("MIXED_DOUBLES");
	let selectedIds = $state<string[]>([]);
	let proposal = $state<{
		a: [string, string];
		b: [string, string];
		ratingA: number;
		ratingB: number;
	} | null>(null);
	let manualCourtId = $state("");
	let recommending = $state(false);
	let confirming = $state(false);

	function toggleSelect(playerId: string) {
		proposal = null;
		if (selectedIds.includes(playerId)) {
			selectedIds = selectedIds.filter((id) => id !== playerId);
		} else if (selectedIds.length < 4) {
			selectedIds = [...selectedIds, playerId];
		}
	}

	async function previewManual() {
		if (!hostToken || selectedIds.length !== 4) return;
		recommending = true;
		try {
			const p = await api.recommendManualMatch(
				sessionId,
				{
					player_ids: selectedIds as [string, string, string, string],
					format: manualFormat,
				},
				hostToken,
			);
			proposal = {
				a: p.team_a,
				b: p.team_b,
				ratingA: p.team_a_rating,
				ratingB: p.team_b_rating,
			};
			if (!manualCourtId && availableCourts.length)
				manualCourtId = availableCourts[0].id;
		} catch (err) {
			toast.error(
				err instanceof ApiError
					? err.message
					: "Could not build a proposal.",
			);
		} finally {
			recommending = false;
		}
	}

	// ---- manual builder: drag a player onto the other team to swap ----
	// Pointer Events (not HTML5 drag-and-drop) so this works the same with
	// touch as with a mouse — the majority of usage here is on a phone.
	let dragging = $state<{ id: string; from: "a" | "b" } | null>(null);
	let dragPos = $state({ x: 0, y: 0 });
	let hoverPlayerId = $state<string | null>(null);

	function teamAvgRating(team: readonly string[]): number {
		const known = team
			.map((id) => rating(id))
			.filter((r): r is number => r !== undefined);
		return known.length
			? known.reduce((s, r) => s + r, 0) / known.length
			: 0;
	}

	function swapPlayers(idA: string, idB: string) {
		if (!proposal || idA === idB) return;
		const inA = (id: string) => proposal!.a.includes(id);
		const replace = (
			team: [string, string],
			from: string,
			to: string,
		): [string, string] =>
			team.map((id) => (id === from ? to : id)) as [string, string];
		if (inA(idA) === inA(idB)) return; // only swapping across teams makes sense
		proposal = inA(idA)
			? {
					...proposal,
					a: replace(proposal.a, idA, idB),
					b: replace(proposal.b, idB, idA),
				}
			: {
					...proposal,
					a: replace(proposal.a, idB, idA),
					b: replace(proposal.b, idA, idB),
				};
	}

	function replaceProposalPlayer(
		team: "a" | "b",
		idx: 0 | 1,
		newPlayerId: string,
	) {
		if (!proposal || !newPlayerId) return;
		const currentTeam = [...proposal[team]] as [string, string];
		currentTeam[idx] = newPlayerId;
		proposal = {
			...proposal,
			[team]: currentTeam,
			ratingA: team === "a" ? teamAvgRating(currentTeam) : proposal.ratingA,
			ratingB: team === "b" ? teamAvgRating(currentTeam) : proposal.ratingB,
		};
	}

	function startDrag(e: PointerEvent, id: string, from: "a" | "b") {
		dragging = { id, from };
		dragPos = { x: e.clientX, y: e.clientY };
		window.addEventListener("pointermove", onDragMove);
		window.addEventListener("pointerup", onDragEnd, { once: true });
	}

	function onDragMove(e: PointerEvent) {
		dragPos = { x: e.clientX, y: e.clientY };
		const el = document
			.elementFromPoint(e.clientX, e.clientY)
			?.closest<HTMLElement>("[data-player-slot]");
		hoverPlayerId = el?.dataset.playerSlot ?? null;
	}

	function onDragEnd() {
		window.removeEventListener("pointermove", onDragMove);
		if (dragging && hoverPlayerId) swapPlayers(dragging.id, hoverPlayerId);
		dragging = null;
		hoverPlayerId = null;
	}

	async function confirmManual() {
		if (!hostToken || !proposal || !manualCourtId) return;
		confirming = true;
		try {
			const match = await api.confirmManualMatch(
				sessionId,
				{
					court_id: manualCourtId,
					format: manualFormat,
					team_a: proposal.a,
					team_b: proposal.b,
				},
				hostToken,
			);
			rememberMatchTeams(match.id, proposal.a, proposal.b);
			courtOccupants = {
				...courtOccupants,
				[manualCourtId]: [...proposal.a, ...proposal.b],
			};
			await poll();
			selectedIds = [];
			proposal = null;
			toast.success("Match confirmed. On court!");
		} catch (err) {
			toast.error(
				err instanceof ApiError
					? err.message
					: "Could not confirm the match.",
			);
		} finally {
			confirming = false;
		}
	}

	// ---- self status ----
	let selfBusy = $state(false);

	async function setMyStatus(status: "WAITING" | "BREAK" | "ENDED") {
		if (!token || !myPlayer) return;
		if (status === "ENDED" && !confirm("Leave this session?")) return;
		selfBusy = true;
		try {
			await api.setSessionPlayerStatus(myPlayer.id, status, token);
			toast.info(
				status === "ENDED"
					? "You left the session."
					: status === "BREAK"
						? "Enjoy the break."
						: "You're back in the queue.",
			);
			poll();
		} catch (err) {
			toast.error(
				err instanceof ApiError
					? err.message
					: "Could not update your status.",
			);
		} finally {
			selfBusy = false;
		}
	}

	async function hostEndPlayer(sp: SessionPlayer) {
		if (!hostToken) return;
		if (!confirm(`Mark ${name(sp.player_id)} as left?`)) return;
		try {
			await api.setSessionPlayerStatus(sp.id, "ENDED", hostToken);
			poll();
		} catch (err) {
			toast.error(
				err instanceof ApiError
					? err.message
					: "Could not update that player.",
			);
		}
	}

	// ---- HUD panel (host tools tucked behind a floating control, not
	// inline in the main flow — see the pop-art/HUD design pass) ----
	let hudOpen = $state(false);
	let matchesOpen = $state(false);
	let activeTool = $state<"match" | "courts" | "invite" | "club">("match");

	function openHud(
		tool: "match" | "courts" | "invite" | "club" = activeTool,
	) {
		activeTool = tool;
		hudOpen = true;
	}

	// Swipe-up-from-the-bottom-edge, in addition to tapping the FAB — an
	// alternate, gesture-first way in for a host who's already down there.
	// Scoped to the bottom strip so it doesn't hijack ordinary scrolling.
	let swipeStartY = $state<number | null>(null);

	function onWindowPointerDown(e: PointerEvent) {
		if (!isHost || hudOpen) return;
		if (e.clientY > window.innerHeight - 90) swipeStartY = e.clientY;
	}

	function onWindowPointerUp(e: PointerEvent) {
		if (swipeStartY !== null && swipeStartY - e.clientY > 32) openHud();
		swipeStartY = null;
	}

	// ---- share ----
	let club = $derived(session ? identity.club(session.club_id) : undefined);
	let inviteLink = $derived(
		club && typeof window !== "undefined"
			? `${window.location.origin}/join?session=${sessionId}&code=${club.joinCode}`
			: "",
	);
	let clubInviteLink = $derived(
		club && typeof window !== "undefined"
			? `${window.location.origin}/club/${club.id}`
			: "",
	);
	let linkCopied = $state(false);
	let clubLinkCopied = $state(false);
	let clubCodeCopied = $state(false);

	async function copyInvite() {
		try {
			await navigator.clipboard.writeText(inviteLink);
			linkCopied = true;
			setTimeout(() => (linkCopied = false), 1600);
		} catch {
			toast.info(inviteLink);
		}
	}

	async function copyClubInvite() {
		try {
			await navigator.clipboard.writeText(clubInviteLink);
			clubLinkCopied = true;
			setTimeout(() => (clubLinkCopied = false), 1600);
		} catch {
			toast.info(clubInviteLink);
		}
	}

	async function copyClubCode() {
		if (!club) return;
		try {
			await navigator.clipboard.writeText(club.joinCode);
			clubCodeCopied = true;
			setTimeout(() => (clubCodeCopied = false), 1600);
		} catch {
			toast.info(`Join code: ${club.joinCode}`);
		}
	}

	$effect(() => {
		// Ratings show on every player row (Playing/Waiting/Break) and the
		// "You" card regardless of assignment mode, so prefetch for everyone
		// currently visible, not just the manual-picker's waiting list.
		if (!token) return;
		ensureRatings(
			players.map((p) => p.player_id),
			token,
		);
	});

	// ---- relogin ----
	// A device that visited this session before (even in a previous browser
	// session) may still have clubBySession[sessionId] and a cached
	// clubs[clubId] entry — from setSessionClub/cacheClubInfo — even after
	// its own session token has gone missing or stopped working (cleared
	// storage, expired token, backend restart). When that's the case, we can
	// still prefill the join code so getting back in is a one-field form,
	// not a dead end asking someone to track down the host again.
	let rejoinClub = $derived(
		(() => {
			const clubId = $identity.clubBySession[sessionId];
			return clubId ? identity.club(clubId) : undefined;
		})(),
	);
	let rejoinLink = $derived(
		rejoinClub
			? `/join?session=${sessionId}&code=${rejoinClub.joinCode}`
			: `/join?session=${sessionId}`,
	);
</script>

<svelte:head>
	<title>{session?.name || "Session"} — deuce</title>
</svelte:head>

<svelte:window
	onpointerdown={onWindowPointerDown}
	onpointerup={onWindowPointerUp}
/>

<PullToRefresh onrefresh={poll}>
	{#if !token}
		<section class="container rise-in">
			<div class="card">
				<p class="muted">
					{rejoinClub
						? "You're signed out on this device. Enter your name again to get back in — your rating and history pick up right where they left off."
						: "You'll need to join this session to see it."}
				</p>
				<a
					href={rejoinLink}
					class="btn btn-primary"
					style="margin-top:16px;"
				>
					{rejoinClub ? "Sign back in" : "Join this session"}
				</a>
			</div>
		</section>
	{:else if loading}
		<section class="container rise-in">
			<div
				class="skeleton"
				style="height:120px; margin-bottom:16px;"
			></div>
			<div class="skeleton" style="height:220px;"></div>
		</section>
	{:else if !session}
		<section class="container rise-in">
			<div class="card">
				<p class="muted">{loadError ?? "Session not found."}</p>
				<p class="muted small" style="margin-top:8px;">
					If your device got signed out, you can rejoin instead of
					starting over.
				</p>
				<a
					href={rejoinLink}
					class="btn btn-ghost"
					style="margin-top:12px;">Sign back in</a
				>
			</div>
		</section>
	{:else}
		<section class="container-wide rise-in">
			<header class="head spread">
				<div>
					<div class="row">
						<h1>{session.name || "Session"}</h1>
						<span
							class="badge dot"
							class:badge-live={session.status === "ACTIVE"}
						>
							{statusLabel(session.status)}
						</span>
					</div>
					<p class="muted small">
						{session.assignment_mode === "AUTOMATIC"
							? "Automatic assignment"
							: "Manual assignment"}
					</p>
				</div>
				<div class="row">
					<a
						href="/club/{session.club_id}"
						class="btn btn-ghost btn-sm">Club</a
					>
					{#if isHost}
						{#if session.status === "NOT_STARTED"}
							<button
								class="btn btn-primary"
								onclick={startSession}
								disabled={busy}>Start session</button
							>
						{:else if session.status === "ACTIVE"}
							<button
								class="btn btn-ghost"
								onclick={endSession}
								disabled={busy}>End session</button
							>
						{/if}
					{/if}
				</div>
			</header>

			{#if loadError}
				<p class="hint-warn small" style="margin-top:4px;">
					{loadError}
				</p>
			{/if}

			{#if myPlayer}
				<div class="card card-pop card-tight me-card row spread">
					<div class="row">
						<span class="badge badge-accent dot">You</span>
						<span>{statusLabel(myPlayer.status)}</span>
						{#if rating(myPlayer.player_id)}
							<span class="mono muted small"
								>{Math.round(rating(myPlayer.player_id) ?? 0)} rating</span
							>
						{/if}
						{#if myPlayer.status === "WAITING"}
							<span class="mono muted small"
								>waiting {mmss(
									liveWaitingSeconds(myPlayer, tick),
								)}</span
							>
						{/if}
					</div>
					<div class="row">
						{#if myPlayer.status === "WAITING"}
							<button
								class="btn btn-ghost btn-sm"
								onclick={() => setMyStatus("BREAK")}
								disabled={selfBusy}>Take a break</button
							>
						{:else if myPlayer.status === "BREAK"}
							<button
								class="btn btn-primary btn-sm"
								onclick={() => setMyStatus("WAITING")}
								disabled={selfBusy}>Back in</button
							>
						{/if}
						{#if myPlayer.status === "WAITING" || myPlayer.status === "BREAK"}
							<button
								class="btn btn-danger btn-sm"
								onclick={() => setMyStatus("ENDED")}
								disabled={selfBusy}>Leave</button
							>
						{/if}
					</div>
				</div>
			{/if}

			<div class="spread section-header" style="margin-top:20px; margin-bottom:12px;">
				<button type="button" class="section-toggle-btn" onclick={() => (courtsCollapsed = !courtsCollapsed)}>
					<span class="toggle-arrow" class:collapsed={courtsCollapsed}>▼</span>
					<h2 class="section-title">Courts ({courts.length})</h2>
				</button>
			</div>
			{#if !courtsCollapsed}
				<div class="courts-grid">
					{#each courts as court (court.id)}
						{@const activeMatch = courtActiveMatch(court.id)}
						{#if activeMatch}
							<a
								href="/match/{activeMatch.id}?session={sessionId}"
								class="card card-tight court-card court-card-link"
								title="Click to view active match"
							>
								<div class="spread">
									<strong>{court.name}</strong>
									<span class="badge dot badge-live">PLAYING →</span>
								</div>
								<div class="occupants">
									{#if courtOccupants[court.id] && courtOccupants[court.id].length}
										{#each courtOccupants[court.id] as pid}
											<span class="pick-chip">{name(pid)}</span>
										{/each}
									{/if}
								</div>
							</a>
						{:else}
							<div class="card card-tight court-card">
								<div class="spread">
									<strong>{court.name}</strong>
									<span class="badge dot">{statusLabel(court.status)}</span>
								</div>
								<div class="occupants">
									{#if courtOccupants[court.id] && courtOccupants[court.id].length}
										{#each courtOccupants[court.id] as pid}
											<span class="pick-chip">{name(pid)}</span>
										{/each}
									{:else}
										<span class="faint small">Court available</span>
									{/if}
								</div>
							</div>
						{/if}
					{/each}
					{#if courts.length === 0}
						<p class="muted small">No courts yet.</p>
					{/if}
				</div>
			{/if}

			<div class="spread section-header" style="margin-top:24px; margin-bottom:12px;">
				<button type="button" class="section-toggle-btn" onclick={() => (playersCollapsed = !playersCollapsed)}>
					<span class="toggle-arrow" class:collapsed={playersCollapsed}>▼</span>
					<h2 class="section-title">
						Players ({players.length - ended.length})
					</h2>
				</button>
				<div class="row">
					<span class="faint small">View:</span>
					<div class="segmented mini">
						<button
							type="button"
							class:active={showRankMode === "both"}
							onclick={() => (showRankMode = "both")}>Both</button
						>
						<button
							type="button"
							class:active={showRankMode === "rank"}
							onclick={() => (showRankMode = "rank")}>Rank</button
						>
						<button
							type="button"
							class:active={showRankMode === "rating"}
							onclick={() => (showRankMode = "rating")}>Rating</button
						>
					</div>
				</div>
			</div>
			{#if !playersCollapsed}
				<div class="players-grid">
					<div class="stack">
						<span class="faint small col-label"
							>Playing — {playing.length}</span
						>
						{#each playing as p (p.id)}
							<div class="prow card-tight">
								<a
									class="pname"
									href="/player/{p.player_id}?session={sessionId}"
								>
									{name(p.player_id)}
									{#if isGuest(p.player_id)}<span
											class="guest-badge">guest</span
										>{/if}
								</a>
								<span class="row">
									{#if rating(p.player_id) != null}
										<span class="mono faint small row gap-xs">
											{#if showRankMode === "both" || showRankMode === "rank"}
												<span class="badge {getRankBadgeClass(getRank(rating(p.player_id)))}"
													>{getRank(rating(p.player_id))}</span
												>
											{/if}
											{#if showRankMode === "both" || showRankMode === "rating"}
												<span>{Math.round(rating(p.player_id) ?? 0)}</span>
											{/if}
											{#if isHost}
												<button
													class="link-btn"
													onclick={() => openEditRating(p.player_id)}
													title="Edit rating/rank">
													<svg width="800px" height="800px" viewBox="0 0 15 15" fill="none" xmlns="http://www.w3.org/2000/svg">
														<path d="M0.5 10.5L0.146447 10.1464L0 10.2929V10.5H0.5ZM10.5 0.5L10.8536 0.146447C10.6583 -0.0488155 10.3417 -0.0488155 10.1464 0.146447L10.5 0.5ZM14.5 4.5L14.8536 4.85355C15.0488 4.65829 15.0488 4.34171 14.8536 4.14645L14.5 4.5ZM4.5 14.5V15H4.70711L4.85355 14.8536L4.5 14.5ZM0.5 14.5H0C0 14.7761 0.223858 15 0.5 15L0.5 14.5ZM0.853553 10.8536L10.8536 0.853553L10.1464 0.146447L0.146447 10.1464L0.853553 10.8536ZM10.1464 0.853553L14.1464 4.85355L14.8536 4.14645L10.8536 0.146447L10.1464 0.853553ZM14.1464 4.14645L4.14645 14.1464L4.85355 14.8536L14.8536 4.85355L14.1464 4.14645ZM4.5 14H0.5V15H4.5V14ZM1 14.5V10.5H0V14.5H1Z" fill="#000000"/>
														</svg>
													</button
												>
											{/if}
										</span>
									{/if}
									<span class="mono faint small"
										>{p.wins}W {p.losses}L</span
									>
								</span>
							</div>
						{/each}
						{#if playing.length === 0}<p class="faint small">
								Nobody on court yet.
							</p>{/if}
					</div>

					<div class="stack">
						<span class="faint small col-label"
							>Waiting — {waiting.length}</span
						>
						{#each waiting as p (p.id)}
							<div class="prow card-tight">
								<a
									class="pname"
									href="/player/{p.player_id}?session={sessionId}"
								>
									{name(p.player_id)}
									{#if isGuest(p.player_id)}<span
											class="guest-badge">guest</span
										>{/if}
								</a>
								<span class="row">
									{#if rating(p.player_id) != null}
										<span class="mono faint small row gap-xs">
											{#if showRankMode === "both" || showRankMode === "rank"}
												<span class="badge {getRankBadgeClass(getRank(rating(p.player_id)))}"
													>{getRank(rating(p.player_id))}</span
												>
											{/if}
											{#if showRankMode === "both" || showRankMode === "rating"}
												<span>{Math.round(rating(p.player_id) ?? 0)}</span>
											{/if}
											{#if isHost}
												<button
													class="link-btn"
													onclick={() => openEditRating(p.player_id)}
													title="Edit rating/rank">
													<svg width="800px" height="800px" viewBox="0 0 15 15" fill="none" xmlns="http://www.w3.org/2000/svg">
														<path d="M0.5 10.5L0.146447 10.1464L0 10.2929V10.5H0.5ZM10.5 0.5L10.8536 0.146447C10.6583 -0.0488155 10.3417 -0.0488155 10.1464 0.146447L10.5 0.5ZM14.5 4.5L14.8536 4.85355C15.0488 4.65829 15.0488 4.34171 14.8536 4.14645L14.5 4.5ZM4.5 14.5V15H4.70711L4.85355 14.8536L4.5 14.5ZM0.5 14.5H0C0 14.7761 0.223858 15 0.5 15L0.5 14.5ZM0.853553 10.8536L10.8536 0.853553L10.1464 0.146447L0.146447 10.1464L0.853553 10.8536ZM10.1464 0.853553L14.1464 4.85355L14.8536 4.14645L10.8536 0.146447L10.1464 0.853553ZM14.1464 4.14645L4.14645 14.1464L4.85355 14.8536L14.8536 4.85355L14.1464 4.14645ZM4.5 14H0.5V15H4.5V14ZM1 14.5V10.5H0V14.5H1Z" fill="#000000"/>
														</svg>
													</button
												>
											{/if}
										</span>
									{/if}
									<span class="mono muted small"
										>{mmss(liveWaitingSeconds(p, tick))}</span
									>
									{#if isHost}
										<button
											class="link-btn"
											onclick={() => hostEndPlayer(p)}
											title="Mark as left">✕</button
										>
									{/if}
								</span>
							</div>
						{/each}
						{#if waiting.length === 0}<p class="faint small">
								Queue is empty.
							</p>{/if}
					</div>

					<div class="stack">
						<span class="faint small col-label"
							>On break — {onBreak.length}</span
						>
						{#each onBreak as p (p.id)}
							<div class="prow card-tight">
								<a
									class="pname"
									href="/player/{p.player_id}?session={sessionId}"
								>
									{name(p.player_id)}
									{#if isGuest(p.player_id)}<span
											class="guest-badge">guest</span
										>{/if}
								</a>
								<span class="row">
									{#if rating(p.player_id) != null}
										<span class="mono faint small row gap-xs">
											{#if showRankMode === "both" || showRankMode === "rank"}
												<span class="badge {getRankBadgeClass(getRank(rating(p.player_id)))}"
													>{getRank(rating(p.player_id))}</span
												>
											{/if}
											{#if showRankMode === "both" || showRankMode === "rating"}
												<span>{Math.round(rating(p.player_id) ?? 0)}</span>
											{/if}
											{#if isHost}
												<button
													class="link-btn"
													onclick={() => openEditRating(p.player_id)}
													title="Edit rating/rank">
													<svg width="800px" height="800px" viewBox="0 0 15 15" fill="none" xmlns="http://www.w3.org/2000/svg">
														<path d="M0.5 10.5L0.146447 10.1464L0 10.2929V10.5H0.5ZM10.5 0.5L10.8536 0.146447C10.6583 -0.0488155 10.3417 -0.0488155 10.1464 0.146447L10.5 0.5ZM14.5 4.5L14.8536 4.85355C15.0488 4.65829 15.0488 4.34171 14.8536 4.14645L14.5 4.5ZM4.5 14.5V15H4.70711L4.85355 14.8536L4.5 14.5ZM0.5 14.5H0C0 14.7761 0.223858 15 0.5 15L0.5 14.5ZM0.853553 10.8536L10.8536 0.853553L10.1464 0.146447L0.146447 10.1464L0.853553 10.8536ZM10.1464 0.853553L14.1464 4.85355L14.8536 4.14645L10.8536 0.146447L10.1464 0.853553ZM14.1464 4.14645L4.14645 14.1464L4.85355 14.8536L14.8536 4.85355L14.1464 4.14645ZM4.5 14H0.5V15H4.5V14ZM1 14.5V10.5H0V14.5H1Z" fill="#000000"/>
														</svg>
													</button
												>
											{/if}
										</span>
									{/if}
									{#if isHost}
										<button
											class="link-btn"
											onclick={() => hostEndPlayer(p)}
											title="Mark as left">✕</button
										>
									{/if}
								</span>
							</div>
						{/each}
						{#if onBreak.length === 0}<p class="faint small">
								Nobody on a break.
							</p>{/if}
					</div>
				</div>
			{/if}

			<div class="spread section-header" style="margin-top:24px; margin-bottom:12px;">
				<button type="button" class="section-toggle-btn" onclick={() => (currentMatchesCollapsed = !currentMatchesCollapsed)}>
					<span class="toggle-arrow" class:collapsed={currentMatchesCollapsed}>▼</span>
					<h2 class="section-title">Current Matches ({currentMatches.length})</h2>
				</button>
			</div>
			{#if !currentMatchesCollapsed}
				{#if currentMatches.length}
					<div class="stack">
						{#each currentMatches as m (m.id)}
							<a href="/match/{m.id}?session={sessionId}" class="card match-card match-card-live" style="margin-bottom:8px;">
								<div class="spread">
									<div class="row">
										<span class="badge dot badge-live">Playing</span>
										{#if courts.find(c => c.id === m.court_id)}
											<strong>{courts.find(c => c.id === m.court_id)?.name}</strong>
										{/if}
									</div>
									<span class="btn btn-ghost btn-sm">View Match →</span>
								</div>
								{#if $matchTeams[m.id]}
									<div class="proposal" style="margin-top:12px;">
										<div class="team">
											<span class="faint small">Team A</span>
											<p>
												{name($matchTeams[m.id].a[0])} &amp; {name($matchTeams[m.id].a[1])}
											</p>
										</div>
										<span class="vs">vs</span>
										<div class="team">
											<span class="faint small">Team B</span>
											<p>
												{name($matchTeams[m.id].b[0])} &amp; {name($matchTeams[m.id].b[1])}
											</p>
										</div>
									</div>
								{/if}
							</a>
						{/each}
					</div>
				{:else}
					<p class="muted small">No active matches currently playing.</p>
				{/if}
			{/if}

			<div class="spread section-header" style="margin-top:24px; margin-bottom:12px;">
				<button type="button" class="section-toggle-btn" onclick={() => (historicalMatchesCollapsed = !historicalMatchesCollapsed)}>
					<span class="toggle-arrow" class:collapsed={historicalMatchesCollapsed}>▼</span>
					<h2 class="section-title">Historical Matches ({historicalMatches.length})</h2>
				</button>
			</div>
			{#if !historicalMatchesCollapsed}
				{#if historicalMatches.length}
					<div class="stack">
						{#each historicalMatches as m, i (m.id)}
							<a
								href="/match/{m.id}?session={sessionId}"
								class="card card-tight match-row row spread"
							>
								<span>
									Match - {historicalMatches.length - i}
									{#if $matchTeams[m.id]}
										<span class="muted small">
											(
											{name($matchTeams[m.id].a[0])} &amp; {name(
												$matchTeams[m.id].a[1],
											)}
										</span>
										vs
										<span class="muted small">
											{name($matchTeams[m.id].b[0])} &amp; {name(
												$matchTeams[m.id].b[1],
											)}
											)
										</span>
									{/if}
								</span>

								<span class="row">
									<span class="mono"
										>{m.score_a}–{m.score_b}</span
									>
									<span class="badge">Finished</span>
								</span>
							</a>
						{/each}
					</div>
				{:else}
					<p class="muted small">No completed matches yet.</p>
				{/if}
			{/if}
		</section>

		<!-- Rendered outside the section above (not nested inside a `.rise-in`
	     entrance-animated ancestor) so `position: fixed` here is actually
	     relative to the viewport — a finished CSS transform animation still
	     establishes a containing block even at its `translateY(0)` end state. -->
		{#if isHost}
			<button
				class="hud-fab"
				onclick={() => openHud()}
				aria-label="Open host tools"
				title="Tap or swipe up from the bottom edge"
			>
				<svg
					viewBox="0 0 24 24"
					width="26"
					height="26"
					fill="none"
					stroke="currentColor"
					stroke-width="2"
					stroke-linecap="round"
					stroke-linejoin="round"
				>
					<path d="M6 3h9a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H6" />
					<path d="M6 3v18" />
					<circle
						cx="15"
						cy="12"
						r="1"
						fill="currentColor"
						stroke="none"
					/>
				</svg>
			</button>
		{/if}

		{#if hudOpen}
			<div
				class="hud-backdrop"
				onclick={() => (hudOpen = false)}
				role="presentation"
			></div>
			<div class="hud-panel rise-in">
				<div class="tool-tabs" role="tablist" aria-label="Host tools">
					<button
						class:active={activeTool === "match"}
						onclick={() => (activeTool = "match")}>Match</button
					>
					<button
						class:active={activeTool === "courts"}
						onclick={() => (activeTool = "courts")}>Courts</button
					>
					<button
						class:active={activeTool === "invite"}
						onclick={() => (activeTool = "invite")}>Invite</button
					>
					<button
						class:active={activeTool === "club"}
						onclick={() => (activeTool = "club")}>Club</button
					>
				</div>
				<div class="hud-panel-body">
					{#if activeTool === "invite"}
						<div class="stack">
							{#if club}
								<div class="card card-tight card-pop">
									<div class="spread" style="margin-bottom:4px;">
										<strong style="font-size:0.85rem;">Permanent Club Link</strong>
										<span class="badge badge-accent">Best for members</span>
									</div>
									<p class="muted small">
										Members bookmark this link to view current &amp; future sessions anytime.
									</p>
									<button
										type="button"
										class="link-text mono"
										class:copied={clubLinkCopied}
										use:longpress={copyClubInvite}
										aria-label="Hold to copy permanent club link"
									>
										{clubInviteLink}
									</button>
								</div>
								<div class="card card-tight">
									<p class="muted small">
										Direct session link · hold to copy
									</p>
									<button
										type="button"
										class="link-text mono"
										class:copied={linkCopied}
										use:longpress={copyInvite}
										aria-label="Hold to copy session link"
									>
										{inviteLink}
									</button>
								</div>
								<div class="card builder">
									<h2 class="section-title">Guest players</h2>
									<p class="faint small">
										Add players who didn't join via the
										link. Paste names, one per line or
										comma-separated.
									</p>
									<textarea
										class="input"
										bind:value={guestNames}
										rows="3"
										placeholder="Alice&#10;Bob&#10;Charlie"
									></textarea>
									<button
										class="btn btn-ghost btn-sm"
										style="margin-top:8px;"
										onclick={parseGuestNames}
										disabled={!guestNames.trim()}
									>
										Add names
									</button>

									{#if guestDraft.length}
										<div
											class="stack"
											style="margin-top:12px;"
										>
											{#each guestDraft as g, i (g.name)}
												<div
													class="row spread guest-row"
												>
													<span class="pname"
														>{g.name}</span
													>
													<div class="row">
														<div
															class="segmented mini"
															role="radiogroup"
															aria-label="Gender for {g.name}"
														>
															<button
																type="button"
																class:active={g.gender ===
																	"MALE"}
																onclick={() =>
																	setGuestGender(
																		i,
																		"MALE",
																	)}>M</button
															>
															<button
																type="button"
																class:active={g.gender ===
																	"FEMALE"}
																onclick={() =>
																	setGuestGender(
																		i,
																		"FEMALE",
																	)}>F</button
															>
														</div>
														<button
															class="link-btn"
															onclick={() =>
																removeGuest(i)}
															title="Remove"
															>✕</button
														>
													</div>
												</div>
											{/each}
										</div>
										<button
											class="btn btn-primary"
											style="margin-top:12px;"
											onclick={addGuests}
											disabled={addingGuests}
										>
											{addingGuests
												? "Adding…"
												: `Add ${guestDraft.length} guest${guestDraft.length > 1 ? "s" : ""}`}
										</button>
									{/if}
								</div>
							{:else}
								<p class="muted small">Loading club info…</p>
							{/if}
						</div>
					{:else if activeTool === "club"}
						<div class="stack">
							{#if club}
								<div class="card card-tight">
									<span class="faint small">{club.name}</span>
									<p class="muted small">
										Join code · hold to copy
									</p>
									<button
										type="button"
										class="club-code mono"
										class:copied={clubCodeCopied}
										use:longpress={copyClubCode}
										aria-label="Hold to copy join code"
									>
										{club.joinCode}
									</button>
								</div>
								<a
									href="/club/{session.club_id}"
									class="btn btn-ghost btn-block"
									>Manage club &amp; members →</a
								>
							{:else}
								<p class="muted small">Loading club info…</p>
							{/if}
						</div>
					{:else if activeTool === "courts"}
						<div class="stack">
							<form onsubmit={addCourt} class="row add-court">
								<input
									class="input"
									placeholder="Court name (e.g. Court 3)"
									bind:value={newCourtName}
									maxlength="40"
								/>
								<button
									class="btn btn-ghost btn-sm"
									disabled={addingCourt ||
										!newCourtName.trim()}>Add court</button
								>
							</form>
							<div class="stack">
								{#each courts as court (court.id)}
									<div class="prow">
										<strong>{court.name}</strong>
										<span
											class="badge dot"
											class:badge-live={court.status ===
												"PLAYING"}
											>{statusLabel(court.status)}</span
										>
									</div>
								{/each}
								{#if courts.length === 0}
									<p class="muted small">No courts yet.</p>
								{/if}
							</div>
						</div>
					{:else if activeTool === "match"}
						{#if session.status !== "FINISHED"}
							<div class="card builder">
								<h2 class="section-title">Matchmaking</h2>
								<div class="field">
									<span
										class="field-label"
										id="assignment-mode-label"
										>Assignment</span
									>
									<div
										class="segmented"
										role="radiogroup"
										aria-labelledby="assignment-mode-label"
									>
										<button
											type="button"
											class:active={session.assignment_mode ===
												"AUTOMATIC"}
											onclick={() =>
												changeMode("AUTOMATIC")}
											disabled={changingMode}
										>
											Automatic
										</button>
										<button
											type="button"
											class:active={session.assignment_mode ===
												"MANUAL"}
											onclick={() => changeMode("MANUAL")}
											disabled={changingMode}
										>
											Manual
										</button>
									</div>
									<p class="faint small">
										{session.assignment_mode === "AUTOMATIC"
											? "The engine picks the next four by rating and wait time."
											: "You pick the four; we suggest fair teams."}
									</p>
								</div>

								{#if session.assignment_mode === "AUTOMATIC"}
									<div class="field">
										<span
											class="field-label"
											id="auto-fill-label"
											>Auto-fill empty courts</span
										>
										<div
											class="segmented"
											role="radiogroup"
											aria-labelledby="auto-fill-label"
										>
											<button
												type="button"
												class:active={session.auto_fill_enabled}
												onclick={() =>
													setAutoFill(true)}
												disabled={togglingAutoFill}
											>
												On
											</button>
											<button
												type="button"
												class:active={!session.auto_fill_enabled}
												onclick={() =>
													setAutoFill(false)}
												disabled={togglingAutoFill}
											>
												Off
											</button>
										</div>
										<p class="faint small">
											{session.auto_fill_enabled
												? "Open courts fill themselves; no tap needed."
												: "You'll tap Generate match for each open court."}
										</p>
									</div>
								{/if}
							</div>

							<div class="card builder">
								<h2 class="section-title">Next match</h2>
								{#if availableCourts.length === 0}
									<p class="muted small">
										No open courts right now.
									</p>
								{:else if waiting.length < 4}
									<p class="muted small">
										Need at least 4 waiting players ({waiting.length}
										now).
									</p>
								{:else if session.assignment_mode === "AUTOMATIC"}
									<div class="stack">
										<div class="row wrap">
											<select
												class="select"
												bind:value={autoCourtId}
											>
												{#each availableCourts as c}
													<option value={c.id}
														>{c.name}</option
													>
												{/each}
											</select>
										</div>
										<button
											class="btn btn-primary"
											onclick={generateAuto}
											disabled={generating}
										>
											{generating
												? "Assigning…"
												: "Generate match"}
										</button>
										<p class="faint small">
											Picks the fairest 4 from the queue
											automatically.
										</p>
									</div>
								{:else}
									<div class="stack">
										<p class="muted small">
											Pick 4 waiting players ({selectedIds.length}/4).
										</p>
										<div class="pick-grid">
											{#each waiting as p (p.id)}
												<button
													type="button"
													class="pick-chip"
													class:selected={selectedIds.includes(
														p.player_id,
													)}
													disabled={!selectedIds.includes(
														p.player_id,
													) &&
														selectedIds.length >= 4}
													onclick={() =>
														toggleSelect(
															p.player_id,
														)}
												>
													{name(p.player_id)}
													{#if rating(p.player_id)}<span
															class="mono faint"
														>
															{Math.round(
																rating(
																	p.player_id,
																) ?? 0,
															)}</span
														>{/if}
												</button>
											{/each}
										</div>

										<button
											class="btn btn-ghost"
											onclick={previewManual}
											disabled={selectedIds.length !==
												4 || recommending}
										>
											{recommending
												? "Balancing…"
												: "Preview teams"}
										</button>

										{#if proposal}
											<p class="faint small">
												Drag to swap teams, or choose a member from the dropdown to replace.
											</p>
											<div class="proposal rise-in">
												<div
													class="team"
													class:drop-hover={hoverPlayerId &&
														proposal.a.includes(
															hoverPlayerId,
														) &&
														dragging?.from === "b"}
												>
													<span class="faint small"
														>Team A</span
													>
													<div class="team-players stack gap-xs">
														{#each proposal.a as id, idx (id)}
															<div class="row gap-xs wrap">
																<button
																	type="button"
																	class="drag-chip"
																	class:dragging-self={dragging?.id ===
																		id}
																	data-player-slot={id}
																	onpointerdown={(
																		e,
																	) =>
																		startDrag(
																			e,
																			id,
																			"a",
																		)}
																>
																	{name(id)}
																</button>
																<select
																	class="select select-sm mini-replace-select"
																	value={id}
																	onchange={(e) =>
																		replaceProposalPlayer(
																			"a",
																			idx as 0 | 1,
																			e.currentTarget.value,
																		)}
																	title="Replace member"
																>
																	<option value={id}>Replace member…</option>
																	{#each waiting.filter((wp) => !proposal?.a.includes(wp.player_id) && !proposal?.b.includes(wp.player_id)) as wp}
																		<option value={wp.player_id}>
																			Replace with {name(wp.player_id)} [{getRank(rating(wp.player_id))}]
																		</option>
																	{/each}
																</select>
															</div>
														{/each}
													</div>
													<span
														class="mono faint small"
														>avg {teamAvgRating(
															proposal.a,
														).toFixed(0)}</span
													>
												</div>
												<span class="vs">vs</span>
												<div
													class="team"
													class:drop-hover={hoverPlayerId &&
														proposal.b.includes(
															hoverPlayerId,
														) &&
														dragging?.from === "a"}
												>
													<span class="faint small"
														>Team B</span
													>
													<div class="team-players stack gap-xs">
														{#each proposal.b as id, idx (id)}
															<div class="row gap-xs wrap">
																<button
																	type="button"
																	class="drag-chip"
																	class:dragging-self={dragging?.id ===
																		id}
																	data-player-slot={id}
																	onpointerdown={(
																		e,
																	) =>
																		startDrag(
																			e,
																			id,
																			"b",
																		)}
																>
																	{name(id)}
																</button>
																<select
																	class="select select-sm mini-replace-select"
																	value={id}
																	onchange={(e) =>
																		replaceProposalPlayer(
																			"b",
																			idx as 0 | 1,
																			e.currentTarget.value,
																		)}
																	title="Replace member"
																>
																	<option value={id}>Replace member…</option>
																	{#each waiting.filter((wp) => !proposal?.a.includes(wp.player_id) && !proposal?.b.includes(wp.player_id)) as wp}
																		<option value={wp.player_id}>
																			Replace with {name(wp.player_id)} [{getRank(rating(wp.player_id))}]
																		</option>
																	{/each}
																</select>
															</div>
														{/each}
													</div>
													<span
														class="mono faint small"
														>avg {teamAvgRating(
															proposal.b,
														).toFixed(0)}</span
													>
												</div>
											</div>
											<div class="row wrap">
												<select
													class="select"
													bind:value={manualCourtId}
												>
													{#each availableCourts as c}
														<option value={c.id}
															>{c.name}</option
														>
													{/each}
												</select>
												<button
													class="btn btn-primary"
													onclick={confirmManual}
													disabled={confirming}
												>
													{confirming
														? "Confirming…"
														: "Confirm match"}
												</button>
											</div>
										{/if}
									</div>
								{/if}
							</div>
						{:else}
							<p class="muted small">
								Start the session before assigning matches.
							</p>
						{/if}
					{/if}
				</div>
			</div>
		{/if}

		{#if dragging}
			<div
				class="drag-ghost"
				style="left:{dragPos.x}px; top:{dragPos.y}px;"
			>
				{name(dragging.id)}
			</div>
		{/if}

		{#if editRatingPlayerId}
			<div
				class="modal-backdrop"
				onclick={() => (editRatingPlayerId = null)}
				role="presentation"
			></div>
			<div class="card card-pop modal-dialog rise-in">
				<div class="spread" style="margin-bottom: 14px;">
					<h3>Update Rating for {name(editRatingPlayerId)}</h3>
					<button
						type="button"
						class="link-btn"
						onclick={() => (editRatingPlayerId = null)}
						title="Close">✕</button
					>
				</div>
				<div class="field">
					<label for="rank-selector">Select Rank Category</label>
					<select
						id="rank-selector"
						class="select"
						bind:value={editRankVal}
						onchange={(e) => onRankSelectChange(e.currentTarget.value)}
					>
						{#each RANK_TIERS as tier}
							<option value={tier.rank}>{tier.label}</option>
						{/each}
					</select>
				</div>
				<div class="field">
					<label for="rating-input">Rating Value</label>
					<input
						id="rating-input"
						type="number"
						class="input mono"
						bind:value={editRatingVal}
					/>
				</div>
				<div class="row spread modal-actions" style="margin-top: 18px; gap: 10px;">
					<button
						class="btn btn-ghost"
						style="flex: 1;"
						onclick={() => (editRatingPlayerId = null)}>Cancel</button
					>
					<button
						class="btn btn-primary"
						style="flex: 1;"
						onclick={savePlayerRating}
						disabled={savingRating}
					>
						{savingRating ? "Saving…" : "Save Rating"}
					</button>
				</div>
			</div>
		{/if}
	{/if}
</PullToRefresh>

<style>
	.head h1 {
		font-size: 1.5rem;
		font-weight: 800;
		letter-spacing: -0.01em;
		display: inline;
		margin-right: 10px;
	}

	.small {
		font-size: 0.8rem;
	}

	.field-label {
		font-size: 0.8rem;
		color: var(--text-dim);
		font-weight: 600;
		letter-spacing: 0.01em;
	}

	.me-card {
		margin-top: 16px;
	}

	.invite-card,
	.builder {
		margin-bottom: 16px;
	}

	/* ---------- host tools HUD (floating, out of the main flow) ---------- */

	.hud-fab {
		position: fixed;
		right: 20px;
		bottom: calc(20px + env(safe-area-inset-bottom, 0px));
		z-index: 60;
		width: 56px;
		height: 56px;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		border: 2px solid var(--border);
		border-radius: 100px;
		background: var(--accent);
		color: var(--accent-contrast);
		box-shadow: var(--shadow-pink);
		cursor: pointer;
		touch-action: pan-y;
	}

	.tool-tabs {
		display: grid;
		grid-template-columns: repeat(4, 1fr);
		gap: 6px;
		margin: 12px 0 4px;
		padding: 4px;
		border: 2px solid var(--border);
		border-radius: var(--radius-sm);
		background: var(--bg-elevated-2);
	}

	.tool-tabs button {
		border: 0;
		border-radius: 6px;
		background: transparent;
		color: var(--text-dim);
		padding: 10px 6px;
		font-size: 0.78rem;
		font-weight: 800;
		text-transform: uppercase;
		cursor: pointer;
	}

	.tool-tabs button.active {
		background: var(--accent);
		color: var(--accent-contrast);
	}

	.club-code {
		display: block;
		width: 100%;
		background: none;
		border: none;
		padding: 0;
		margin-top: 4px;
		text-align: left;
		font-size: 1.5rem;
		font-weight: 800;
		letter-spacing: 0.08em;
		color: var(--accent);
		cursor: pointer;
		transition: opacity 0.15s ease;
	}

	.club-code.copied {
		opacity: 0.5;
	}

	.hud-backdrop {
		position: fixed;
		inset: 0;
		background: rgba(0, 0, 0, 0.6);
		z-index: 70;
	}

	.hud-panel {
		position: fixed;
		left: 0;
		right: 0;
		bottom: 0;
		z-index: 71;
		max-height: 82vh;
		overflow-y: auto;
		background: var(--bg-elevated);
		border-top: 2px solid var(--accent);
		border-radius: var(--radius-lg) var(--radius-lg) 0 0;
		box-shadow: 0 -6px 0 0 rgba(198, 255, 92, 0.12);
		padding: 16px 18px calc(20px + env(safe-area-inset-bottom, 0px));
	}

	.hud-panel-head {
		position: sticky;
		top: 0;
		background: var(--bg-elevated);
		padding-bottom: 12px;
		margin-bottom: 4px;
		border-bottom: 1px solid var(--border-soft);
	}

	.hud-panel-body {
		padding-top: 14px;
	}

	@media (min-width: 720px) {
		.hud-panel {
			left: auto;
			width: 380px;
			border-left: 2px solid var(--accent);
			border-radius: var(--radius-lg) 0 0 var(--radius-lg);
		}
	}
	.link-text {
		max-width: 260px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		background: none;
		border: none;
		padding: 0;
		margin-top: 4px;
		text-align: left;
		font-weight: 800;
		letter-spacing: 0.08em;
		color: var(--accent);
		cursor: pointer;
		transition: opacity 0.15s ease;
	}

	.link-text.copied {
		opacity: 0.5;
	}

	.section-title {
		font-size: 0.95rem;
		font-weight: 700;
		margin: 26px 0 10px;
	}

	.courts-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
		gap: 10px;
	}

	.court-card .occupants {
		margin-top: 6px;
	}

	.occupants {
		display: grid;
		grid-template-columns: repeat(2, 1fr);
		grid-template-rows: repeat(2, 1fr);
		gap: 0.1rem;
	}

	.add-court {
		margin-top: 10px;
	}

	.add-court .input {
		flex: 1;
	}

	.players-grid {
		display: grid;
		grid-template-columns: repeat(3, 1fr);
		gap: 20px;
	}

	.col-label {
		text-transform: uppercase;
		letter-spacing: 0.06em;
	}

	.prow {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 10px 12px;
		background: var(--bg-elevated);
		border: 1px solid var(--border-soft);
		border-radius: var(--radius-sm);
		font-size: 0.9rem;
	}

	.pname {
		color: var(--text);
	}

	.pname:hover {
		color: var(--accent);
	}

	.guest-badge {
		display: inline-block;
		margin-left: 8px;
		padding: 1px 7px;
		border: 1px solid var(--border);
		border-radius: 100px;
		color: var(--text-faint);
		font-size: 0.68rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		vertical-align: middle;
	}

	.guest-row {
		padding: 8px 10px;
		background: var(--bg-elevated);
		border: 1px solid var(--border-soft);
		border-radius: var(--radius-sm);
	}

	.segmented.mini button {
		padding: 4px 9px;
		font-size: 0.72rem;
	}

	.link-btn {
		background: none;
		border: none;
		color: var(--text-faint);
		cursor: pointer;
		padding: 2px 4px;
		font-size: 0.85rem;
	}

	.link-btn:hover {
		color: var(--danger);
	}

	.pick-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(110px, 1fr));
		gap: 8px;
	}

	.pick-chip {
		border: 1px solid var(--border);
		background: var(--bg-elevated);
		color: var(--text);
		padding: 10px;
		border-radius: var(--radius-sm);
		cursor: pointer;
		font-size: 0.85rem;
		text-align: left;
	}

	.pick-chip.selected {
		border-color: var(--accent);
		background: color-mix(in srgb, var(--accent) 14%, var(--bg-elevated));
	}

	.pick-chip:disabled {
		opacity: 0.4;
		cursor: not-allowed;
	}

	.proposal {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 16px;
		padding: 14px;
		background: var(--bg-elevated-2);
		border-radius: var(--radius-sm);
		text-align: center;
	}

	.proposal .team {
		flex: 1;
		padding: 8px;
		border-radius: var(--radius-sm);
		border: 2px solid transparent;
		transition:
			border-color 0.12s ease,
			background-color 0.12s ease;
	}

	.proposal .team.drop-hover {
		border-color: var(--accent);
		background: color-mix(in srgb, var(--accent) 12%, transparent);
	}

	.team-players {
		display: flex;
		flex-direction: column;
		gap: 6px;
		margin: 6px 0;
	}

	.drag-chip {
		border: 2px solid var(--border);
		background: var(--bg-elevated);
		color: var(--text);
		padding: 8px 10px;
		border-radius: var(--radius-sm);
		font-weight: 700;
		font-size: 0.85rem;
		cursor: grab;
		touch-action: none;
		user-select: none;
	}

	.drag-chip.dragging-self {
		opacity: 0.35;
	}

	.drag-ghost {
		position: fixed;
		z-index: 80;
		transform: translate(-50%, -50%);
		background: var(--accent);
		color: var(--accent-contrast);
		border: 2px solid var(--accent-contrast);
		padding: 8px 14px;
		border-radius: 100px;
		font-weight: 800;
		font-size: 0.85rem;
		pointer-events: none;
		box-shadow: var(--shadow-sm);
	}

	.vs {
		color: var(--text-faint);
		font-size: 0.8rem;
	}

	.row.wrap {
		flex-wrap: wrap;
	}

	.match-row {
		transition: border-color 0.15s ease;
	}

	.match-row:hover {
		border-color: var(--text-dim);
	}

	.court-card-link {
		cursor: pointer;
		transition: transform 0.15s ease, border-color 0.15s ease, box-shadow 0.15s ease;
	}
	.court-card-link:hover {
		border-color: var(--accent);
		transform: translateY(-2px);
		box-shadow: var(--shadow-sm);
	}
	.modal-backdrop {
		position: fixed;
		inset: 0;
		background: rgba(0, 0, 0, 0.75);
		backdrop-filter: blur(4px);
		z-index: 199;
	}
	.modal-dialog {
		position: fixed;
		top: 50%;
		left: 50%;
		transform: translate(-50%, -50%);
		z-index: 200;
		width: calc(100% - 32px);
		max-width: 420px;
		max-height: calc(100vh - 40px);
		overflow-y: auto;
		background: var(--bg-elevated);
		border: 2px solid var(--accent);
		box-shadow: 0 12px 36px rgba(0, 0, 0, 0.85), var(--shadow);
		padding: 22px;
		border-radius: var(--radius-lg);
	}
	.mini-replace-select {
		padding: 4px 8px;
		font-size: 0.78rem;
		max-width: 140px;
		background: var(--bg);
	}
	.gap-xs {
		gap: 6px;
	}

	@media (max-width: 520px) {
		.modal-dialog {
			top: auto;
			bottom: 16px;
			left: 16px;
			right: 16px;
			width: auto;
			transform: none;
			max-height: 85vh;
			padding: 18px;
		}
	}

	@media (max-width: 720px) {
		.players-grid {
			grid-template-columns: 1fr;
		}
	}
</style>
