import type {
	Club,
	Court,
	Gender,
	Match,
	MatchFormat,
	MatchProposal,
	MatchSummary,
	PlayerAuth,
	Player,
	Rating,
	Session,
	SessionDetail,
	SessionPlayer,
	SessionPlayerStatus,
	AssignmentMode,
	Team,
	ClubRole,
	Member
} from './types';
import { getDeviceId } from './device';

const BASE_URL = import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:8080/api/v1';

export class ApiError extends Error {
	code: string;
	status: number;

	constructor(code: string, message: string, status: number) {
		super(message);
		this.code = code;
		this.status = status;
	}
}

async function request<T>(
	path: string,
	options: { method?: string; body?: unknown; token?: string } = {}
): Promise<T> {
	const headers: Record<string, string> = {};
	if (options.body !== undefined) headers['Content-Type'] = 'application/json';
	if (options.token) headers['Authorization'] = `Bearer ${options.token}`;

	let res: Response;
	try {
		res = await fetch(`${BASE_URL}${path}`, {
			method: options.method ?? 'GET',
			headers,
			body: options.body !== undefined ? JSON.stringify(options.body) : undefined
		});
	} catch {
		throw new ApiError('NETWORK_ERROR', 'Could not reach the server. Check your connection.', 0);
	}

	if (res.status === 204) return undefined as T;

	// Not every non-2xx response is our JSON error envelope — e.g. Fiber's
	// rate limiter returns a plain-text body ("Too Many Requests") for 429.
	// Parse defensively so a body like that doesn't crash as an uncaught
	// SyntaxError instead of a readable ApiError.
	const text = await res.text();
	let data: unknown;
	try {
		data = text ? JSON.parse(text) : undefined;
	} catch {
		data = undefined;
	}

	if (!res.ok) {
		const body = data as { error?: { code?: string; message?: string } } | undefined;
		const code = body?.error?.code ?? (res.status === 429 ? 'RATE_LIMITED' : 'UNKNOWN_ERROR');
		const message =
			body?.error?.message ??
			(res.status === 429
				? "You're refreshing too fast — wait a few seconds and try again."
				: `Request failed (${res.status})`);
		throw new ApiError(code, message, res.status);
	}

	return data as T;
}

export const api = {
	createClub(payload: { club_name: string; host_display_name: string; host_gender: Gender }) {
		return request<{ club: Club; host: PlayerAuth }>('/clubs/', {
			method: 'POST',
			body: { ...payload, device_id: getDeviceId() }
		});
	},

	joinClub(
		clubId: string,
		payload: { join_code: string; display_name: string; gender: Gender }
	) {
		return request<{ club: Club; you: PlayerAuth }>(`/clubs/${clubId}/join`, {
			method: 'POST',
			body: { ...payload, device_id: getDeviceId() }
		});
	},

	getClub(clubId: string, token: string) {
		return request<Club>(`/clubs/${clubId}`, { token });
	},

	getMyClubRole(clubId: string, token: string) {
		return request<{ role: ClubRole }>(`/clubs/${clubId}/me`, { token });
	},

	listClubMembers(clubId: string, token: string) {
		return request<{ members: Member[] }>(`/clubs/${clubId}/members`, { token });
	},

	promoteMember(clubId: string, playerId: string, token: string) {
		return request<Member>(`/clubs/${clubId}/members/${playerId}/promote`, {
			method: 'POST',
			token
		});
	},

	listClubSessions(clubId: string, token: string) {
		return request<{ sessions: Session[] }>(`/clubs/${clubId}/sessions`, { token });
	},

	createSession(
		token: string,
		payload: { club_id: string; name?: string; assignment_mode?: AssignmentMode }
	) {
		return request<Session>('/sessions/', { method: 'POST', body: payload, token });
	},

	joinSession(
		sessionId: string,
		payload: { join_code: string; display_name: string; gender: Gender }
	) {
		return request<{ session: Session; session_player: SessionPlayer; you: PlayerAuth }>(
			`/sessions/${sessionId}/join`,
			{ method: 'POST', body: { ...payload, device_id: getDeviceId() } }
		);
	},

	getSession(sessionId: string, token: string) {
		return request<SessionDetail>(`/sessions/${sessionId}`, { token });
	},

	startSession(sessionId: string, token: string) {
		return request<Session>(`/sessions/${sessionId}/start`, { method: 'POST', token });
	},

	endSession(sessionId: string, token: string) {
		return request<Session>(`/sessions/${sessionId}/end`, { method: 'POST', token });
	},

	setAssignmentMode(sessionId: string, assignmentMode: AssignmentMode, token: string) {
		return request<Session>(`/sessions/${sessionId}/assignment-mode`, {
			method: 'PATCH',
			body: { assignment_mode: assignmentMode },
			token
		});
	},

	setAutoFillEnabled(sessionId: string, autoFillEnabled: boolean, token: string) {
		return request<Session>(`/sessions/${sessionId}/auto-fill`, {
			method: 'PATCH',
			body: { auto_fill_enabled: autoFillEnabled },
			token
		});
	},

	createCourt(sessionId: string, name: string, token: string) {
		return request<Court>(`/sessions/${sessionId}/courts`, {
			method: 'POST',
			body: { name },
			token
		});
	},

	registerGuests(
		sessionId: string,
		payload: { guests: { display_name: string; gender: Gender }[] },
		token: string
	) {
		return request<{ session_players: SessionPlayer[] }>(`/sessions/${sessionId}/guests`, {
			method: 'POST',
			body: payload,
			token
		});
	},

	generateMatch(
		sessionId: string,
		payload: { court_id: string; format: MatchFormat },
		token: string
	) {
		return request<Match>(`/sessions/${sessionId}/matches/generate`, {
			method: 'POST',
			body: payload,
			token
		});
	},

	recommendManualMatch(
		sessionId: string,
		payload: { player_ids: [string, string, string, string]; format: MatchFormat },
		token: string
	) {
		return request<MatchProposal>(`/sessions/${sessionId}/matches/manual/recommend`, {
			method: 'POST',
			body: payload,
			token
		});
	},

	confirmManualMatch(
		sessionId: string,
		payload: {
			court_id: string;
			format: MatchFormat;
			team_a: [string, string];
			team_b: [string, string];
		},
		token: string
	) {
		return request<Match>(`/sessions/${sessionId}/matches/manual/confirm`, {
			method: 'POST',
			body: payload,
			token
		});
	},

	listSessionMatches(sessionId: string, token: string) {
		return request<{ matches: Match[] }>(`/sessions/${sessionId}/matches`, { token });
	},

	setSessionPlayerStatus(
		sessionPlayerId: string,
		status: SessionPlayerStatus,
		token: string
	) {
		return request<SessionPlayer>(`/session-players/${sessionPlayerId}/status`, {
			method: 'PATCH',
			body: { status },
			token
		});
	},

	getMatch(matchId: string, token: string) {
		return request<Match>(`/matches/${matchId}`, { token });
	},

	startMatch(matchId: string, token: string) {
		return request<Match>(`/matches/${matchId}/start`, { method: 'POST', token });
	},

	finishMatch(matchId: string, payload: { score_a: number; score_b: number }, token: string) {
		return request<Match>(`/matches/${matchId}/finish`, {
			method: 'POST',
			body: payload,
			token
		});
	},

	getPlayer(playerId: string, token: string) {
		return request<Player>(`/players/${playerId}`, { token });
	},

	getPlayerRating(playerId: string, token: string) {
		return request<Rating>(`/players/${playerId}/rating`, { token });
	},

	listPlayerMatches(
		playerId: string,
		token: string,
		params: { limit?: number; offset?: number } = {}
	) {
		const qs = new URLSearchParams();
		if (params.limit) qs.set('limit', String(params.limit));
		if (params.offset) qs.set('offset', String(params.offset));
		const suffix = qs.toString() ? `?${qs.toString()}` : '';
		return request<{ matches: MatchSummary[] }>(`/players/${playerId}/matches${suffix}`, {
			token
		});
	}
};

export type { Team };
