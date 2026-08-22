import { writable, get } from 'svelte/store';
import { api } from '../api';
import type { Player } from '../types';

export interface ClubIdentity {
	id: string;
	name: string;
	joinCode: string;
	hostPlayerId: string;
	// Only set on the device that originally created the club (see
	// rememberClub). A promoted co-host's device has everything else here
	// (via cacheClubInfo) but no hostToken/hostName of its own — it uses its
	// own session token instead (see hostTokenForClub).
	hostToken?: string;
	hostName?: string;
}

export interface SessionIdentity {
	sessionId: string;
	clubId?: string;
	token: string;
	sessionPlayerId: string;
	player: Player;
}

// A club-level identity from rejoining via join code directly (no session
// involved) — e.g. a host or member whose local identity went missing but
// whose device is still recognized server-side (see internal/device), so
// rejoining hands back the *same* player/role rather than a brand-new one.
export interface ClubMembership {
	token: string;
	player: Player;
}

interface IdentityState {
	clubs: Record<string, ClubIdentity>;
	clubMemberships: Record<string, ClubMembership>;
	sessions: Record<string, SessionIdentity>;
	clubBySession: Record<string, string>;
	sessionsByClub: Record<string, string[]>;
	// Clubs where this device's own player (not the club's original creator)
	// was confirmed HOST via GET /clubs/:clubId/me — a promoted co-host has
	// no local "I created this" flag, so that check is what recognizes them.
	// Populated by ensureCoHostChecked(); never assume false permanently,
	// since promotion can happen at any time after this device last checked.
	coHostClubs: Record<string, boolean>;
	/** Most recently created/visited/joined club — "Back to your club" aims
	 * here instead of an arbitrary (possibly deleted) first key. */
	lastClubId?: string;
	/** The most recent display name + gender used by this device — lets
	 * join/rejoin forms prefill themselves so a relogin is one tap, not a
	 * re-typing exercise. */
	lastProfile?: { displayName: string; gender: 'MALE' | 'FEMALE' };
}

/** Shallow copy of obj without key — avoids leaking stale entries. */
function omitKey<T extends object>(obj: T, key: string): T {
	if (!(key in (obj as Record<string, unknown>))) return obj;
	const next = { ...(obj as Record<string, unknown>) };
	delete next[key];
	return next as T;
}

const STORAGE_KEY = 'deuce:identity:v1';
const isBrowser = typeof window !== 'undefined';
const EMPTY: IdentityState = {
	clubs: {},
	clubMemberships: {},
	sessions: {},
	clubBySession: {},
	sessionsByClub: {},
	coHostClubs: {}
};

function load(): IdentityState {
	if (!isBrowser) return { ...EMPTY };
	try {
		const raw = localStorage.getItem(STORAGE_KEY);
		if (!raw) return { ...EMPTY };
		return { ...EMPTY, ...JSON.parse(raw) };
	} catch {
		return { ...EMPTY };
	}
}

const store = writable<IdentityState>(load());

if (isBrowser) {
	store.subscribe((state) => {
		try {
			localStorage.setItem(STORAGE_KEY, JSON.stringify(state));
		} catch {
			// storage unavailable (private mode, quota) — identity just won't persist
		}
	});
}

export const identity = {
	subscribe: store.subscribe,

	rememberClub(
		club: { id: string; name: string; join_code: string; host_player_id: string },
		hostToken: string,
		hostName: string
	) {
		store.update((s) => ({
			...s,
			clubs: {
				...s.clubs,
				[club.id]: {
					id: club.id,
					name: club.name,
					joinCode: club.join_code,
					hostPlayerId: club.host_player_id,
					hostToken,
					hostName
				}
			},
			lastClubId: club.id
		}));
	},

	/**
	 * Caches a club's public info (name, join code) for THIS device,
	 * regardless of whether it created the club — merges into any existing
	 * entry rather than clobbering a creator's hostToken/hostName. This is
	 * what lets a promoted co-host's device resolve `identity.club(clubId)`
	 * (e.g. for the session page's invite link) after simply loading the
	 * club page once.
	 */
	cacheClubInfo(club: { id: string; name: string; join_code: string; host_player_id: string }) {
		store.update((s) => ({
			...s,
			clubs: {
				...s.clubs,
				[club.id]: {
					...s.clubs[club.id],
					id: club.id,
					name: club.name,
					joinCode: club.join_code,
					hostPlayerId: club.host_player_id
				}
			},
			lastClubId: club.id
		}));
	},

	/**
	 * Records a club-level identity obtained by rejoining via join code
	 * directly (api.joinClub), not through a session. Used to recover from
	 * this device's saved identity going missing/invalid — the device_id
	 * link server-side (see internal/device) means rejoining hands back the
	 * same player and role, so this is a real relogin, not a fresh join.
	 */
	rememberClubMembership(clubId: string, token: string, player: Player) {
		store.update((s) => ({
			...s,
			clubMemberships: { ...s.clubMemberships, [clubId]: { token, player } },
			lastClubId: clubId,
			lastProfile: { displayName: player.display_name, gender: player.gender }
		}));
	},

	rememberProfile(displayName: string, gender: 'MALE' | 'FEMALE') {
		if (!displayName) return;
		store.update((s) => ({ ...s, lastProfile: { displayName, gender } }));
	},

	rememberSession(sessionId: string, entry: Omit<SessionIdentity, 'sessionId'>) {
		store.update((s) => ({
			...s,
			sessions: { ...s.sessions, [sessionId]: { sessionId, ...entry } },
			clubBySession: entry.clubId
				? { ...s.clubBySession, [sessionId]: entry.clubId }
				: s.clubBySession,
			lastProfile: { displayName: entry.player.display_name, gender: entry.player.gender }
		}));
	},

	setSessionClub(sessionId: string, clubId: string) {
		store.update((s) => {
			// Visiting a session marks its club as the current one, even when
			// nothing else changes — "Back to your club" follows activity.
			const withLast = { ...s, lastClubId: clubId };
			if (s.clubBySession[sessionId] === clubId) return withLast;
			const existing = s.sessionsByClub[clubId] ?? [];
			return {
				...withLast,
				clubBySession: { ...s.clubBySession, [sessionId]: clubId },
				sessionsByClub: existing.includes(sessionId)
					? s.sessionsByClub
					: { ...s.sessionsByClub, [clubId]: [...existing, sessionId] }
			};
		});
	},

	/**
	 * Drops every local trace of a club — cached info, membership token,
	 * session mappings, co-host flag. Called when the server confirms the
	 * club no longer exists (404), so links like "Back to your club" stop
	 * steering people at dead entries.
	 */
	forgetClub(clubId: string) {
		store.update((s) => {
			const sessionIds = s.sessionsByClub[clubId] ?? [];
			let sessions = s.sessions;
			let clubBySession = s.clubBySession;
			for (const sid of sessionIds) {
				sessions = omitKey(sessions, sid);
				clubBySession = omitKey(clubBySession, sid);
			}
			return {
				...s,
				clubs: omitKey(s.clubs, clubId),
				clubMemberships: omitKey(s.clubMemberships, clubId),
				sessionsByClub: omitKey(s.sessionsByClub, clubId),
				coHostClubs: omitKey(s.coHostClubs, clubId),
				sessions,
				clubBySession,
				lastClubId: s.lastClubId === clubId ? undefined : s.lastClubId
			};
		});
	},

	club(clubId: string): ClubIdentity | undefined {
		return get(store).clubs[clubId];
	},

	session(sessionId: string): SessionIdentity | undefined {
		return get(store).sessions[sessionId];
	},

	sessionsForClub(clubId: string): string[] {
		return get(store).sessionsByClub[clubId] ?? [];
	},

	isHostOfClub(clubId: string): boolean {
		const s = get(store);
		return !!s.clubs[clubId]?.hostToken || !!s.coHostClubs[clubId];
	},

	isHostOfSession(sessionId: string): boolean {
		const s = get(store);
		const clubId = s.clubBySession[sessionId];
		return !!clubId && identity.isHostOfClub(clubId);
	},

	/** Any token this device holds for the club — its own player's session
	 * token (from whichever session it joined the club through), a direct
	 * club-membership rejoin token, or the creator's host token. Works for
	 * reads regardless of role. */
	tokenForClub(clubId: string): string | undefined {
		const s = get(store);
		if (s.clubs[clubId]?.hostToken) return s.clubs[clubId].hostToken;
		for (const sess of Object.values(s.sessions)) {
			if (sess.clubId === clubId) return sess.token;
		}
		return s.clubMemberships[clubId]?.token;
	},

	/** Host-only-action token for the club: the creator's token, or — if
	 * this device's own player was promoted — this device's own token
	 * (from a session or a direct club-membership rejoin). */
	hostTokenForClub(clubId: string): string | undefined {
		const s = get(store);
		if (s.clubs[clubId]?.hostToken) return s.clubs[clubId].hostToken;
		if (!s.coHostClubs[clubId]) return undefined;
		for (const sess of Object.values(s.sessions)) {
			if (sess.clubId === clubId) return sess.token;
		}
		return s.clubMemberships[clubId]?.token;
	},

	/** Best available bearer token to read/act on a session: own player token first, else club host token. */
	tokenForSession(sessionId: string): string | undefined {
		const s = get(store);
		const own = s.sessions[sessionId]?.token;
		if (own) return own;
		const clubId = s.clubBySession[sessionId];
		return clubId ? identity.tokenForClub(clubId) : undefined;
	},

	/** Host-only-action token: the club creator's token, or — if this device's
	 * own player was promoted — this device's own session token. */
	hostTokenForSession(sessionId: string): string | undefined {
		const clubId = get(store).clubBySession[sessionId];
		return clubId ? identity.hostTokenForClub(clubId) : undefined;
	},

	markCoHost(clubId: string) {
		store.update((s) => ({ ...s, coHostClubs: { ...s.coHostClubs, [clubId]: true } }));
	},

	/**
	 * Checks the server for whether this device's own player (via its
	 * session token) is a HOST of clubId, and caches the result. Skips the
	 * request if we're already known to be host some other way. Call this
	 * on load/refresh of any page that needs an accurate isHost — it's the
	 * only way a promoted co-host's own device recognizes its new status.
	 */
	async ensureCoHostChecked(clubId: string, token: string | undefined) {
		if (!token) return;
		const s = get(store);
		if (s.clubs[clubId]?.hostToken || s.coHostClubs[clubId]) return;
		try {
			const { role } = await api.getMyClubRole(clubId, token);
			if (role === 'HOST') identity.markCoHost(clubId);
		} catch {
			// not a member, network hiccup, etc. — silently stay non-host
		}
	},

	/** Any bearer token this device holds — auth just needs a valid token, not a specific one. */
	anyToken(): string | undefined {
		const s = get(store);
		const ownSession = Object.values(s.sessions)[0]?.token;
		if (ownSession) return ownSession;
		return Object.values(s.clubs)[0]?.hostToken;
	},

	/** The session identity (if any) whose player is this playerId — used to find "my" token. */
	sessionForPlayer(playerId: string): SessionIdentity | undefined {
		return Object.values(get(store).sessions).find((s) => s.player.id === playerId);
	},

	/** This device's own session identity (if any) for the given club — used
	 * to find "my own" player/name when this device is a co-host rather than
	 * the club's original creator. */
	sessionForClub(clubId: string): SessionIdentity | undefined {
		return Object.values(get(store).sessions).find((s) => s.clubId === clubId);
	},

	forget() {
		store.set({ ...EMPTY });
	}
};
