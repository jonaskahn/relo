import { cubicOut } from 'svelte/easing';
import type { TransitionConfig } from 'svelte/transition';

// Every popover and menu panel opens the same way: it starts four pixels
// above its anchor and drops in. On close the config reverses, because a
// Svelte transition is bidirectional.

/** The open transition every popover and menu panel shares: a four-pixel drop with a fade, which
 *  reverses on close. An operator who asked for reduced motion gets a plain fade. */
export function popoverMotionConfig(reducedMotion: boolean): TransitionConfig {
	if (reducedMotion) {
		return {
			duration: 140,
			css: (t) => `opacity: ${t}`
		};
	}

	return {
		duration: 160,
		easing: cubicOut,
		css: (t) => `opacity: ${t}; transform: translateY(${(1 - t) * -4}px)`
	};
}

/** The shared pop-in every popover and tooltip uses. */
export function popoverMotion(_node: HTMLElement): TransitionConfig {
	return popoverMotionConfig(window.matchMedia('(prefers-reduced-motion: reduce)').matches);
}
