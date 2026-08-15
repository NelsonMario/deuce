import { writable } from 'svelte/store';

export type ToastKind = 'info' | 'success' | 'error';

export interface Toast {
	id: number;
	kind: ToastKind;
	message: string;
}

let counter = 0;
export const toasts = writable<Toast[]>([]);

function push(kind: ToastKind, message: string, ttl = 3600) {
	const id = ++counter;
	toasts.update((list) => [...list, { id, kind, message }]);
	setTimeout(() => {
		toasts.update((list) => list.filter((t) => t.id !== id));
	}, ttl);
}

export const toast = {
	info: (message: string) => push('info', message),
	success: (message: string) => push('success', message),
	error: (message: string) => push('error', message, 5000),
	dismiss: (id: number) => toasts.update((list) => list.filter((t) => t.id !== id))
};
