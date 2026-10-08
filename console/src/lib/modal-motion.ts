import { cubicOut } from 'svelte/easing';
import type { TransitionConfig } from 'svelte/transition';

/** The control a modal grew out of. */
export type ModalOrigin = {
	x: number;
	y: number;
};

/** Where that control sat on screen. */
export type ModalRect = {
	left: number;
	top: number;
	width: number;
	height: number;
};

/** Picks the modal travel: grow from the control, rise from the sheet edge, or swap in place. */
export type ModalMotionMode = 'desktop' | 'mobile' | 'reduced';

const ORIGIN_MAX_AGE_MS = 2_000;
const DESKTOP_MIN_WIDTH = 640;

let lastOrigin: (ModalOrigin & { capturedAt: number }) | null = null;
let trackerUsers = 0;

function activeElementOrigin(): ModalOrigin | null {
	if (!(document.activeElement instanceof HTMLElement)) return null;
	const rect = document.activeElement.getBoundingClientRect();
	return {
		x: rect.left + rect.width / 2,
		y: rect.top + rect.height / 2
	};
}

function rememberOrigin(origin: ModalOrigin | null) {
	lastOrigin = origin === null ? null : { ...origin, capturedAt: Date.now() };
}

function onPointerDown(event: PointerEvent) {
	rememberOrigin({ x: event.clientX, y: event.clientY });
}

function onClick(event: MouseEvent) {
	if (event.detail === 0) rememberOrigin(activeElementOrigin());
}

function onKeyDown() {
	// Keyboard shortcuts do not have a trigger position. Clear a recent
	// pointer so they use the modal's centred fallback instead.
	lastOrigin = null;
}

/** Records the last control a pointer pressed, so a modal knows what to grow out of. */
export function trackModalOrigin(): () => void {
	if (trackerUsers++ === 0) {
		window.addEventListener('pointerdown', onPointerDown, true);
		window.addEventListener('click', onClick, true);
		window.addEventListener('keydown', onKeyDown, true);
	}

	return () => {
		trackerUsers = Math.max(0, trackerUsers - 1);
		if (trackerUsers !== 0) return;
		window.removeEventListener('pointerdown', onPointerDown, true);
		window.removeEventListener('click', onClick, true);
		window.removeEventListener('keydown', onKeyDown, true);
		lastOrigin = null;
	};
}

function takeRecentOrigin(): ModalOrigin | null {
	const origin = lastOrigin;
	lastOrigin = null;
	if (origin === null || Date.now() - origin.capturedAt > ORIGIN_MAX_AGE_MS) return null;
	return origin;
}

/** Turns the origin and the modal's own box into the transform origin the grow starts from. */
export function originFor(origin: ModalOrigin | null, rect: ModalRect): string {
	if (origin === null || rect.width <= 0 || rect.height <= 0) return 'center';
	const x = Math.min(rect.width, Math.max(0, origin.x - rect.left));
	const y = Math.min(rect.height, Math.max(0, origin.y - rect.top));
	return `${x}px ${y}px`;
}

/** Picks the travel from the viewport width and the motion preference. */
export function modalMotionMode(width: number, reducedMotion: boolean): ModalMotionMode {
	if (reducedMotion) return 'reduced';
	return width < DESKTOP_MIN_WIDTH ? 'mobile' : 'desktop';
}

function drawerOut(t: number): number {
	// cubic-bezier(0.32, 0.72, 0, 1), solved for y at the supplied x.
	let parameter = t;
	for (let index = 0; index < 6; index += 1) {
		const inverse = 1 - parameter;
		const x =
			3 * inverse * inverse * parameter * 0.32 +
			3 * inverse * parameter * parameter * 0 +
			parameter * parameter * parameter;
		const slope =
			3 * inverse * inverse * 0.32 +
			6 * inverse * parameter * (0 - 0.32) +
			3 * parameter * parameter * (1 - 0);
		if (Math.abs(slope) < 0.0001) break;
		parameter -= (x - t) / slope;
		parameter = Math.min(1, Math.max(0, parameter));
	}
	const inverse = 1 - parameter;
	return (
		3 * inverse * inverse * parameter * 0.72 +
		3 * inverse * parameter * parameter +
		parameter * parameter * parameter
	);
}

/** The moment a modal's body swaps content while the modal itself stays put: a step change, a
 *  view becoming its editor.
 *  The operator sees one surface change what it shows rather than watching a panel travel, so
 *  the rise is 6px and under reduced motion the swap is a fade. */
export function stepSwap(_node: HTMLElement): TransitionConfig {
	if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
		return {
			duration: 140,
			css: (t) => `opacity: ${t}`
		};
	}

	return {
		duration: 180,
		easing: cubicOut,
		css: (t) => `opacity: ${t}; transform: translateY(${(1 - t) * 6}px)`
	};
}

type ModalMotionParams = {
	startY?: () => number;
};

/** The shared modal transition. */
export function modalMotion(node: HTMLElement, params: ModalMotionParams = {}): TransitionConfig {
	const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
	const mode = modalMotionMode(window.innerWidth, reducedMotion);

	if (mode === 'reduced') {
		return {
			duration: 140,
			css: (t) => `opacity: ${t}`
		};
	}

	if (mode === 'mobile') {
		const startY = Math.max(0, params.startY?.() ?? 0);
		return {
			duration: 260,
			easing: drawerOut,
			css: (t) => `opacity: ${t}; transform: translateY(calc(${startY * t}px + ${(1 - t) * 100}%))`
		};
	}

	node.style.transformOrigin = originFor(takeRecentOrigin(), node.getBoundingClientRect());
	return {
		duration: 180,
		easing: cubicOut,
		css: (t) => `opacity: ${t}; transform: scale(${0.92 + 0.08 * t})`
	};
}

type DrawerMotionParams = {
	startX?: () => number;
};

/** Slides the phone nav drawer in from the left edge:
 *  260ms on the shared drawer curve, a fade only under reduced motion. */
export function drawerEnter(_node: HTMLElement): TransitionConfig {
	if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
		return {
			duration: 140,
			css: (t) => `opacity: ${t}`
		};
	}

	return {
		duration: 260,
		easing: drawerOut,
		css: (t) => `transform: translateX(${(1 - t) * -100}%)`
	};
}

/** Slides the drawer back out a touch quicker than it came in. startX continues an outro begun
 *  by a drag-release from the finger's offset, the way the sheet's startY does; anything else
 *  leaves from fully open. */
export function drawerLeave(node: HTMLElement, params: DrawerMotionParams = {}): TransitionConfig {
	if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
		return {
			duration: 140,
			css: (t) => `opacity: ${t}`
		};
	}

	const startX = Math.min(0, params.startX?.() ?? 0);
	return {
		duration: 220,
		easing: drawerOut,
		css: (t) => `transform: translateX(calc(${startX * t}px - ${(1 - t) * 100}%))`
	};
}
