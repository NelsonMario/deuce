export function mmss(totalSeconds: number): string {
	const s = Math.max(0, Math.floor(totalSeconds));
	const m = Math.floor(s / 60);
	const r = s % 60;
	return `${m}:${String(r).padStart(2, '0')}`;
}

export function formatTime(iso?: string | null): string {
	if (!iso) return '—';
	const d = new Date(iso);
	if (Number.isNaN(d.getTime())) return '—';
	return d.toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' });
}

const STATUS_LABELS: Record<string, string> = {
	NOT_STARTED: 'Not started',
	ACTIVE: 'Active',
	FINISHED: 'Finished',
	WAITING: 'Waiting',
	PLAYING: 'Playing',
	BREAK: 'On break',
	ENDED: 'Left',
	AVAILABLE: 'Open',
	CREATED: 'Created'
};

export function statusLabel(status: string): string {
	return STATUS_LABELS[status] ?? status;
}

export function genderLabel(gender: string): string {
	return gender === 'MALE' ? 'Male' : gender === 'FEMALE' ? 'Female' : gender;
}

export function initials(name: string): string {
	const parts = name.trim().split(/\s+/).filter(Boolean);
	if (parts.length === 0) return '?';
	if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase();
	return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase();
}
