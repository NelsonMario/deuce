const LONGPRESS_MS = 420;

/**
 * Svelte action: fires `onpress` after holding the element for LONGPRESS_MS
 * without moving. Triggers a light haptic vibration if supported (Apple iOS/Android).
 */
export function longpress (node: HTMLElement, onpress: () => void) {
	let timer: ReturnType<typeof setTimeout> | null = null;
	let startX = 0;
	let startY = 0;

	function clear () {
		if (timer) clearTimeout(timer);
		timer = null;
	}

	function start (e: PointerEvent) {
		startX = e.clientX;
		startY = e.clientY;
		clear();
		timer = setTimeout(() => {
			if (typeof navigator !== 'undefined' && navigator.vibrate) {
				try {
					navigator.vibrate(40);
				} catch {
					// Ignore if forbidden
				}
			}
			onpress();
		}, LONGPRESS_MS);
	}

	function move (e: PointerEvent) {
		if (!timer) return;
		if (Math.hypot(e.clientX - startX, e.clientY - startY) > 10) clear();
	}

	node.addEventListener('pointerdown', start);
	node.addEventListener('pointermove', move);
	node.addEventListener('pointerup', clear);
	node.addEventListener('pointerleave', clear);
	node.addEventListener('pointercancel', clear);

	return {
		destroy () {
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
 * past `thresholdPx` without much horizontal drift.
 */
export function swipeUp (node: HTMLElement, onswipeup: () => void, thresholdPx = 24) {
	let startX = 0;
	let startY = 0;
	let tracking = false;

	function start (e: PointerEvent) {
		tracking = true;
		startX = e.clientX;
		startY = e.clientY;
	}

	function end (e: PointerEvent) {
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
		destroy () {
			node.removeEventListener('pointerdown', start);
			node.removeEventListener('pointerup', end);
		}
	};
}

/**
 * Svelte action: fires `onDismiss` when pulling down past `thresholdPx`
 * (iOS sheet gesture).
 */
export function swipeDownSheet (node: HTMLElement, onDismiss: () => void, thresholdPx = 36) {
	let startY = 0;
	let tracking = false;

	function start (e: PointerEvent) {
		tracking = true;
		startY = e.clientY;
	}

	function end (e: PointerEvent) {
		if (!tracking) return;
		tracking = false;
		if (e.clientY - startY > thresholdPx) {
			onDismiss();
		}
	}

	node.addEventListener('pointerdown', start);
	node.addEventListener('pointerup', end);
	node.addEventListener('pointercancel', () => (tracking = false));

	return {
		destroy () {
			node.removeEventListener('pointerdown', start);
			node.removeEventListener('pointerup', end);
		}
	};
}

export interface PinchOpts {
	onZoomIn?: () => void;
	onZoomOut?: () => void;
	onPinchScale?: (scale: number) => void;
}

/**
 * Svelte action: Apple pinch gesture detection (2-finger touch pinch or trackpad pinch).
 * Pinching out (expanding fingers) triggers `onZoomIn`.
 * Pinching in (closing fingers) triggers `onZoomOut`.
 */
export function pinchZoom (node: HTMLElement, opts: PinchOpts = {}) {
	let initialDist = 0;
	let tracking = false;

	function getTouchDist (e: TouchEvent): number {
		if (e.touches.length < 2) return 0;
		const t1 = e.touches[0];
		const t2 = e.touches[1];
		return Math.hypot(t1.clientX - t2.clientX, t1.clientY - t2.clientY);
	}

	function onTouchStart (e: TouchEvent) {
		if (e.touches.length === 2) {
			tracking = true;
			initialDist = getTouchDist(e);
		}
	}

	function onTouchMove (e: TouchEvent) {
		if (!tracking || e.touches.length < 2 || initialDist === 0) return;
		const currentDist = getTouchDist(e);
		const ratio = currentDist / initialDist;
		opts.onPinchScale?.(ratio);

		if (ratio > 1.25) {
			tracking = false;
			if (typeof navigator !== 'undefined' && navigator.vibrate) navigator.vibrate(30);
			opts.onZoomIn?.();
		} else if (ratio < 0.75) {
			tracking = false;
			if (typeof navigator !== 'undefined' && navigator.vibrate) navigator.vibrate(30);
			opts.onZoomOut?.();
		}
	}

	function onTouchEnd () {
		tracking = false;
		initialDist = 0;
	}

	// Trackpad pinch gesture (fires wheel event with Ctrl key)
	function onWheel (e: WheelEvent) {
		if (e.ctrlKey || e.metaKey) {
			e.preventDefault();
			if (e.deltaY < -15) {
				opts.onZoomIn?.();
			} else if (e.deltaY > 15) {
				opts.onZoomOut?.();
			}
		}
	}

	node.addEventListener('touchstart', onTouchStart, { passive: true });
	node.addEventListener('touchmove', onTouchMove, { passive: true });
	node.addEventListener('touchend', onTouchEnd);
	node.addEventListener('wheel', onWheel, { passive: false });

	return {
		destroy () {
			node.removeEventListener('touchstart', onTouchStart);
			node.removeEventListener('touchmove', onTouchMove);
			node.removeEventListener('touchend', onTouchEnd);
			node.removeEventListener('wheel', onWheel);
		}
	};
}

export interface SwipeOpts {
	onSwipeLeft?: () => void;
	onSwipeRight?: () => void;
	thresholdPx?: number;
}

/**
 * Svelte action: iOS-style horizontal swipe on list items or cards.
 */
export function swipeHorizontal (node: HTMLElement, opts: SwipeOpts = {}) {
	let startX = 0;
	let startY = 0;
	let tracking = false;
	const threshold = opts.thresholdPx ?? 40;

	function start (e: PointerEvent) {
		tracking = true;
		startX = e.clientX;
		startY = e.clientY;
	}

	function end (e: PointerEvent) {
		if (!tracking) return;
		tracking = false;
		const dx = e.clientX - startX;
		const dy = e.clientY - startY;
		if (Math.abs(dx) > Math.abs(dy) && Math.abs(dx) > threshold) {
			if (dx > 0) {
				opts.onSwipeRight?.();
			} else {
				opts.onSwipeLeft?.();
			}
		}
	}

	node.addEventListener('pointerdown', start);
	node.addEventListener('pointerup', end);
	node.addEventListener('pointercancel', () => (tracking = false));

	return {
		destroy () {
			node.removeEventListener('pointerdown', start);
			node.removeEventListener('pointerup', end);
		}
	};
}

