const STORAGE_KEY = 'deuce:device_id';

/**
 * A random ID generated once per browser and kept in localStorage. Sent
 * (silently, no UI) with create/join requests so a returning device is
 * recognized as the same player within a club — see backend/internal/device.
 * Scoped to this browser's storage only: a different browser, a cleared
 * site data, or a different device all start over with a fresh device_id.
 */
export function getDeviceId(): string {
	if (typeof window === 'undefined') return '';
	try {
		let id = localStorage.getItem(STORAGE_KEY);
		if (!id) {
			id = crypto.randomUUID();
			localStorage.setItem(STORAGE_KEY, id);
		}
		return id;
	} catch {
		return '';
	}
}
