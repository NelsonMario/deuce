import { writable } from 'svelte/store';

// The API never returns match rosters (see backend/internal/httpapi/handler/match.go —
// MatchDTO has no player fields). We only learn the 4 players at the moment a manual
// match is confirmed (we chose them), so we cache that mapping in memory for as long as
// this tab stays open, to show team composition on the session and match detail pages.
export const matchTeams = writable<Record<string, { a: [string, string]; b: [string, string] }>>({});

export function rememberMatchTeams(matchId: string, a: [string, string], b: [string, string]) {
	matchTeams.update((cache) => ({ ...cache, [matchId]: { a, b } }));
}
