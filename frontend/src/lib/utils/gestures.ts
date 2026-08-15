const LONGPRESS_MS = 480;

/**
 * Svelte action: fires `onpress` after holding the element for LONGPRESS_MS
 * without moving. Used in place of visible "Copy" buttons — hold the value
 * itself to copy it, no explicit control needed.
 */
export function longpress(node: HTMLElement, onpress: () => void) {
	let timer: ReturnType<typeof setTimeout> | null = null;
	let startX = 0;
	let startY = 0;

	function clear() {
		if (timer) clearTimeout(timer);
		timer = null;
	}

	function start(e: PointerEvent) {
		startX = e.clientX;
		startY = e.clientY;
		clear();
		timer = setTimeout(onpress, LONGPRESS_MS);
	}

	function move(e: PointerEvent) {
		if (!timer) return;
		if (Math.hypot(e.clientX - startX, e.clientY - startY) > 10) clear();
	}

	node.addEventListener('pointerdown', start);
	node.addEventListener('pointermove', move);
	node.addEventListener('pointerup', clear);
	node.addEventListener('pointerleave', clear);
	node.addEventListener('pointercancel', clear);

	return {
		destroy() {
			clear();
			node.removeEventListener('pointerdown', start);
			node.removeEventListener('pointermove', move);
			node.removeEventListener('pointerup', clear);
			node.removeEventListener('pointerleave', clear);
			node.removeEventListener('pointercancel', clear);
		}
	};
}

/**
 * Svelte action: fires `onswipeup` when a touch/pointer drag moves upward
 * past `thresholdPx` without much horizontal drift. Used as an alternate,
 * gesture-first way to open the host tools panel (in addition to a tap).
 */
export function swipeUp(node: HTMLElement, onswipeup: () => void, thresholdPx = 24) {
	let startX = 0;
	let startY = 0;
	let tracking = false;

	function start(e: PointerEvent) {
		tracking = true;
		startX = e.clientX;
		startY = e.clientY;
	}

	function end(e: PointerEvent) {
		if (!tracking) return;
		tracking = false;
		const dx = e.clientX - startX;
		const dy = e.clientY - startY;
		if (startY - e.clientY > thresholdPx && Math.abs(dx) < Math.abs(dy)) onswipeup();
	}

	node.addEventListener('pointerdown', start);
	node.addEventListener('pointerup', end);
	node.addEventListener('pointercancel', () => (tracking = false));

	return {
		destroy() {
			node.removeEventListener('pointerdown', start);
			node.removeEventListener('pointerup', end);
		}
	};
}
