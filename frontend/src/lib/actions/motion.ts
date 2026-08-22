// Pointer/scroll motion actions — dependency-free, rAF-batched, and all
// no-ops under prefers-reduced-motion or coarse (touch) pointers where
// hovering doesn't exist.

function reducedMotion(): boolean {
	return (
		typeof window !== 'undefined' &&
		window.matchMedia('(prefers-reduced-motion: reduce)').matches
	);
}

export interface RevealOpts {
	/** Stagger delay in ms. */
	delay?: number;
	/** Initial offset in px. */
	y?: number;
}

/**
 * Scroll-triggered entrance. Pairs with the global `.will-reveal` /
 * `.revealed` classes in app.css — the element slides up + fades in the
 * first time it enters the viewport.
 */
export function reveal(node: HTMLElement, opts: RevealOpts = {}) {
	if (reducedMotion()) return;

	node.classList.add('will-reveal');
	node.style.setProperty('--reveal-delay', `${opts.delay ?? 0}ms`);
	node.style.setProperty('--reveal-y', `${opts.y ?? 22}px`);

	const io = new IntersectionObserver(
		(entries) => {
			for (const entry of entries) {
				if (entry.isIntersecting) {
					node.classList.add('revealed');
					io.disconnect();
				}
			}
		},
		{ threshold: 0.1, rootMargin: '0px 0px -8% 0px' }
	);
	io.observe(node);

	return {
		destroy() {
			io.disconnect();
		}
	};
}

export interface TiltOpts {
	/** Max rotation in degrees. */
	max?: number;
	/** Also expose --spot-x/--spot-y (%) for a pointer-following spotlight. */
	spotlight?: boolean;
}

/**
 * Pointer-tilt (3D card lean) driven entirely through CSS custom props so
 * the element's own transform stays declarative:
 *   .tiltable { transform: perspective(700px) rotateX(var(--tilt-x)) rotateY(var(--tilt-y)); }
 * Mouse only — touch never tilts.
 */
export function tilt(node: HTMLElement, opts: TiltOpts = {}) {
	if (reducedMotion()) return;
	const max = opts.max ?? 5;
	let raf = 0;

	function onMove(e: PointerEvent) {
		if (e.pointerType !== 'mouse') return;
		const r = node.getBoundingClientRect();
		const px = (e.clientX - r.left) / r.width;
		const py = (e.clientY - r.top) / r.height;
		cancelAnimationFrame(raf);
		raf = requestAnimationFrame(() => {
			node.style.setProperty('--tilt-x', `${(0.5 - py) * max}deg`);
			node.style.setProperty('--tilt-y', `${(px - 0.5) * max}deg`);
			if (opts.spotlight !== false) {
				node.style.setProperty('--spot-x', `${px * 100}%`);
				node.style.setProperty('--spot-y', `${py * 100}%`);
				node.classList.add('tilting');
			}
		});
	}

	function onLeave() {
		cancelAnimationFrame(raf);
		node.style.setProperty('--tilt-x', '0deg');
		node.style.setProperty('--tilt-y', '0deg');
		node.classList.remove('tilting');
	}

	node.addEventListener('pointermove', onMove);
	node.addEventListener('pointerleave', onLeave);

	return {
		destroy() {
			cancelAnimationFrame(raf);
			node.removeEventListener('pointermove', onMove);
			node.removeEventListener('pointerleave', onLeave);
		}
	};
}

/**
 * Magnetic hover — the element drifts toward the cursor by a fraction of
 * the offset, then springs back. Desktop-only flourish for hero CTAs and
 * floating action buttons.
 */
export function magnetic(node: HTMLElement, strength = 0.3, maxShift = 14) {
	if (reducedMotion()) return;
	let raf = 0;

	function onMove(e: PointerEvent) {
		if (e.pointerType !== 'mouse') return;
		const r = node.getBoundingClientRect();
		const dx = e.clientX - (r.left + r.width / 2);
		const dy = e.clientY - (r.top + r.height / 2);
		cancelAnimationFrame(raf);
		raf = requestAnimationFrame(() => {
			const x = Math.max(-maxShift, Math.min(maxShift, dx * strength));
			const y = Math.max(-maxShift, Math.min(maxShift, dy * strength));
			node.style.setProperty('--mag-x', `${x}px`);
			node.style.setProperty('--mag-y', `${y}px`);
			node.classList.add('magnetized');
		});
	}

	function onLeave() {
		cancelAnimationFrame(raf);
		node.style.setProperty('--mag-x', '0px');
		node.style.setProperty('--mag-y', '0px');
		node.classList.remove('magnetized');
	}

	node.addEventListener('pointermove', onMove);
	node.addEventListener('pointerleave', onLeave);

	return {
		destroy() {
			cancelAnimationFrame(raf);
			node.removeEventListener('pointermove', onMove);
			node.removeEventListener('pointerleave', onLeave);
		}
	};
}

/**
 * Tracks pointer position over an element as --px/--py (% of box), for CSS
 * effects that follow the cursor (hero spotlight, gradient borders…).
 * Cheap: two custom props per move, rAF-coalesced.
 */
export function trackPointer(node: HTMLElement) {
	if (reducedMotion()) return;
	let raf = 0;

	function onMove(e: PointerEvent) {
		const r = node.getBoundingClientRect();
		const px = ((e.clientX - r.left) / r.width) * 100;
		const py = ((e.clientY - r.top) / r.height) * 100;
		cancelAnimationFrame(raf);
		raf = requestAnimationFrame(() => {
			node.style.setProperty('--px', `${px}%`);
			node.style.setProperty('--py', `${py}%`);
		});
	}

	node.addEventListener('pointermove', onMove);

	return {
		destroy() {
			cancelAnimationFrame(raf);
			node.removeEventListener('pointermove', onMove);
		}
	};
}
