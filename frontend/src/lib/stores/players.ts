import { writable, get } from 'svelte/store';
import { api } from '../api';
import type { Player } from '../types';

export const playerCache = writable<Record<string, Player>>({});

const inFlight = new Set<string>();

export function cachedPlayer (playerId: string): Player | undefined {
	return get(playerCache)[playerId];
}

export async function ensurePlayer (playerId: string, token: string): Promise<Player | undefined> {
	if (get(playerCache)[playerId]) return get(playerCache)[playerId];
	await ensurePlayers([playerId], token);
	return get(playerCache)[playerId];
}

export async function ensurePlayers (playerIds: string[], token: string): Promise<void> {
	const cache = get(playerCache);
	const toFetch = Array.from(new Set(playerIds)).filter((id) => id && !cache[id] && !inFlight.has(id));
	if (toFetch.length === 0) return;

	for (const id of toFetch) {
		inFlight.add(id);
	}

	try {
		const res = await api.getPlayersBatch(toFetch, token);
		const newEntries: Record<string, Player> = {};
		for (const p of res.players) {
			newEntries[p.id] = p;
		}
		playerCache.update((c) => ({ ...c, ...newEntries }));
	} catch {
		// Ignore errors for batch pre-fetch
	} finally {
		for (const id of toFetch) {
			inFlight.delete(id);
		}
	}
}

export const ratingCache = writable<Record<string, number>>({});
const ratingInFlight = new Set<string>();
const ratingFetchedAt = new Map<string, number>();
const RATING_TTL_MS = 20000;

export async function ensureRating (playerId: string, token: string): Promise<number | undefined> {
	const lastFetch = ratingFetchedAt.get(playerId);
	if (lastFetch && Date.now() - lastFetch < RATING_TTL_MS) return get(ratingCache)[playerId];
	await ensureRatings([playerId], token);
	return get(ratingCache)[playerId];
}

export async function ensureRatings (playerIds: string[], token: string): Promise<void> {
	const now = Date.now();
	const cache = get(ratingCache);
	const toFetch = Array.from(new Set(playerIds)).filter((id) => {
		if (!id || ratingInFlight.has(id)) return false;
		const lastFetch = ratingFetchedAt.get(id);
		return !lastFetch || (now - lastFetch >= RATING_TTL_MS);
	});
	if (toFetch.length === 0) return;

	for (const id of toFetch) {
		ratingInFlight.add(id);
	}

	try {
		const res = await api.getPlayerRatingsBatch(toFetch, token);
		const newEntries: Record<string, number> = {};
		for (const r of res.ratings) {
			newEntries[r.player_id] = r.rating;
			ratingFetchedAt.set(r.player_id, now);
		}
		ratingCache.update((c) => ({ ...c, ...newEntries }));
	} catch {
		// Ignore errors for batch rating fetch
	} finally {
		for (const id of toFetch) {
			ratingInFlight.delete(id);
		}
	}
}
