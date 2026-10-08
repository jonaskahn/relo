import { cubicOut } from 'svelte/easing';
import type { TransitionConfig } from 'svelte/transition';

/** The page switch's transition, swapping instantly under reduced motion. */
export function pageMotionConfig(reducedMotion: boolean): TransitionConfig {
	if (reducedMotion) {
		return {
			duration: 120,
			css: (t) => `opacity: ${t}`
		};
	}

	return {
		duration: 180,
		easing: cubicOut,
		css: (t) => `opacity: ${t}; transform: translateY(${(1 - t) * 6}px)`
	};
}

/** The page switch as an in: transition. */
export function pageMotion(_node: HTMLElement): TransitionConfig {
	return pageMotionConfig(window.matchMedia('(prefers-reduced-motion: reduce)').matches);
}
