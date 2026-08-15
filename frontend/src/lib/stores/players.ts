import { writable, get } from 'svelte/store';
import { api } from '../api';
import type { Player } from '../types';

export const playerCache = writable<Record<string, Player>>({});

const inFlight = new Set<string>();

export function cachedPlayer(playerId: string): Player | undefined {
	return get(playerCache)[playerId];
}

export async function ensurePlayer(playerId: string, token: string): Promise<Player | undefined> {
	if (get(playerCache)[playerId] || inFlight.has(playerId)) return get(playerCache)[playerId];
	inFlight.add(playerId);
	try {
		const player = await api.getPlayer(playerId, token);
		playerCache.update((cache) => ({ ...cache, [playerId]: player }));
		return player;
	} catch {
		return undefined;
	} finally {
		inFlight.delete(playerId);
	}
}

export function ensurePlayers(playerIds: string[], token: string) {
	for (const id of playerIds) {
		void ensurePlayer(id, token);
	}
}

export const ratingCache = writable<Record<string, number>>({});
const ratingInFlight = new Set<string>();
const ratingFetchedAt = new Map<string, number>();
const RATING_TTL_MS = 20000;

// Ratings change (a match finishing) far less often than a session polls, so
// we cap how often any one player's rating is refetched — otherwise a
// manual-match builder with 8 waiting players refetches all 8 every poll
// tick, easily blowing past the backend's per-IP rate limit on its own.
export async function ensureRating(playerId: string, token: string): Promise<number | undefined> {
	const lastFetch = ratingFetchedAt.get(playerId);
	if (lastFetch && Date.now() - lastFetch < RATING_TTL_MS) return get(ratingCache)[playerId];
	if (ratingInFlight.has(playerId)) return get(ratingCache)[playerId];
	ratingInFlight.add(playerId);
	try {
		const rating = await api.getPlayerRating(playerId, token);
		ratingCache.update((cache) => ({ ...cache, [playerId]: rating.rating }));
		ratingFetchedAt.set(playerId, Date.now());
		return rating.rating;
	} catch {
		return undefined;
	} finally {
		ratingInFlight.delete(playerId);
	}
}
