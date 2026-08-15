export type Gender = 'MALE' | 'FEMALE';
export type SessionStatus = 'NOT_STARTED' | 'ACTIVE' | 'FINISHED';
export type AssignmentMode = 'AUTOMATIC' | 'MANUAL';
export type SessionPlayerStatus = 'WAITING' | 'PLAYING' | 'BREAK' | 'ENDED';
export type CourtStatus = 'AVAILABLE' | 'PLAYING';
export type MatchFormat = 'MIXED_DOUBLES';
export type MatchStatus = 'CREATED' | 'PLAYING' | 'FINISHED';
export type Team = 'A' | 'B';
export type ClubRole = 'HOST' | 'PLAYER';

export interface Player {
	id: string;
	display_name: string;
	gender: Gender;
	is_guest?: boolean;
}

export interface PlayerAuth {
	player: Player;
	token: string;
	/** True when this device was recognized from a previous join to this club (v2 device identity). */
	returning: boolean;
}

export interface Club {
	id: string;
	name: string;
	host_player_id: string;
	join_code: string;
}

export interface Session {
	id: string;
	club_id: string;
	name: string;
	status: SessionStatus;
	assignment_mode: AssignmentMode;
	auto_fill_enabled: boolean;
	started_at?: string | null;
	ended_at?: string | null;
}

export interface SessionPlayer {
	id: string;
	player_id: string;
	status: SessionPlayerStatus;
	matches_played: number;
	wins: number;
	losses: number;
	waiting_seconds: number;
}

export interface Court {
	id: string;
	name: string;
	status: CourtStatus;
}

export interface SessionDetail {
	session: Session;
	players: SessionPlayer[];
	courts: Court[];
}

export interface Match {
	id: string;
	session_id: string;
	court_id: string;
	format: MatchFormat;
	status: MatchStatus;
	started_at?: string | null;
	ended_at?: string | null;
	score_a?: number | null;
	score_b?: number | null;
	winner?: Team | null;
	players?: string[];
}

export interface MatchProposal {
	team_a: [string, string];
	team_b: [string, string];
	team_a_rating: number;
	team_b_rating: number;
	rating_diff: number;
}

export interface Rating {
	player_id: string;
	rating: number;
}

export interface MatchSummary {
	match_id: string;
	session_id: string;
	format: MatchFormat;
	status: MatchStatus;
	score_a?: number | null;
	score_b?: number | null;
	winner?: Team | null;
}

export interface Member {
	player_id: string;
	role: ClubRole;
	joined_at: string;
}

export interface ApiErrorBody {
	error: { code: string; message: string };
}
