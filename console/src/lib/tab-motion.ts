// The motion every tab strip shares: the glass thumb's spring, the travel
// direction between panes, and the pane swap itself. Kept pure so the
// segmented control, the tab strip, and the tests agree on one mapping.

import { cubicOut } from 'svelte/easing';
import type { TransitionConfig } from 'svelte/transition';

/** tabSpringOptions settles the thumb in about 240ms with a small overshoot, so it glides rather
 *  than snaps.
 *  Reduced motion skips the spring and places the thumb with a hard set instead. */
export const TAB_SPRING = {
	stiffness: 0.25,
	damping: 0.8,
	precision: 0.1
} as const;

/** Reads the operator's motion choice. Guarded for the static prerender, where no media query
 *  exists. */
export function prefersReducedMotion(): boolean {
	if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return false;
	return window.matchMedia('(prefers-reduced-motion: reduce)').matches;
}

/** Names which way a swap travels: forward when the new tab sits after the old one in the strip
 *  order, backward otherwise.
 *  An unknown value travels forward, so a first visit never slides the wrong way. */
export function tabDirection(order: readonly string[], from: string, to: string): 1 | -1 {
	const before = order.indexOf(from);
	const after = order.indexOf(to);
	if (before === -1 || after === -1 || after >= before) return 1;
	return -1;
}

/** Places the thumb under the active segment: its left edge and its width in the track's own
 *  coordinates.
 *  A missing segment parks a zero-width thumb at the track start, so nothing flashes
 *  mid-measure. */
export function thumbTarget(
	rects: readonly { left: number; width: number }[],
	index: number
): {
	left: number;
	width: number;
} {
	const rect = index >= 0 && index < rects.length ? rects[index] : undefined;
	if (!rect) return { left: 0, width: 0 };
	return { left: Math.max(0, rect.left), width: Math.max(0, rect.width) };
}

/** Carries the travel direction and the motion choice to a pane transition. */
export interface TabPaneOptions {
	direction: 1 | -1;
	reducedMotion: boolean;
}

/** Slides the incoming pane 16px from the travel direction with a fade over 200ms.
 *  Reduced motion keeps a short fade with no travel. */
export function tabPaneIn(
	_node: HTMLElement,
	options: TabPaneOptions = { direction: 1, reducedMotion: false }
): TransitionConfig {
	if (options.reducedMotion) {
		return { duration: 120, css: (t) => `opacity: ${t}` };
	}
	return {
		duration: 200,
		easing: cubicOut,
		css: (t) => `opacity: ${t}; transform: translateX(${(1 - t) * 16 * options.direction}px)`
	};
}

/** Slides the outgoing pane 16px past the travel direction with a fade over 160ms.
 *  Reduced motion keeps a short fade with no travel. */
export function tabPaneOut(
	_node: HTMLElement,
	options: TabPaneOptions = { direction: 1, reducedMotion: false }
): TransitionConfig {
	if (options.reducedMotion) {
		return { duration: 120, css: (t) => `opacity: ${t}` };
	}
	return {
		duration: 160,
		easing: cubicOut,
		css: (t) => `opacity: ${t}; transform: translateX(${(1 - t) * -16 * options.direction}px)`
	};
}
