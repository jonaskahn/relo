import { tick } from 'svelte';

// The layout control re-solves the card grid between four and three cards per
// row. Only the cards move, so the change is measured on them before the class
// flip and played back as one transform-only morph: a first/last inversion
// (FLIP) with Web Animations, which touches no inline style and leaves nothing
// to clean up.

/** How long the shared card morph plays, in milliseconds. A caller that gates its next change on
 *  the grid being settled reads the same number. */
export const CARD_MORPH_MS = 200;
/** The curve the shared card morph travels on. */
export const CARD_MORPH_EASING = 'cubic-bezier(.2,.8,.2,1)';
/** Under half a pixel a card "did not move", so the morph is skipped for it: a single-row grid
 *  animates nothing, which is correct rather than a bug. */
export const CARD_MORPH_EPSILON = 0.5;

/** One card's box before and after a layout change. */
export interface CardBox {
	x: number;
	y: number;
	width: number;
	height: number;
}

const CARD_SELECTOR = '[data-slot="card"]';
const DIALOG_SELECTOR = '[role="dialog"]';

/** Reports whether the operator asked for less movement. */
export function reducedMotion(): boolean {
	if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return true;
	return window.matchMedia('(prefers-reduced-motion: reduce)').matches;
}

/** The transform a card needs to look like it is still in its old box: the delta of its corner
 *  and the ratio of its size. */
export function morphTransform(from: CardBox, to: CardBox): string | null {
	const dx = to.x - from.x;
	const dy = to.y - from.y;
	const sx = from.width === 0 ? 1 : to.width / from.width;
	const sy = from.height === 0 ? 1 : to.height / from.height;
	const still =
		Math.abs(dx) < CARD_MORPH_EPSILON &&
		Math.abs(dy) < CARD_MORPH_EPSILON &&
		Math.abs(to.width - from.width) < CARD_MORPH_EPSILON &&
		Math.abs(to.height - from.height) < CARD_MORPH_EPSILON;
	if (still) return null;
	return `translate(${dx}px, ${dy}px) scale(${sx}, ${sy})`;
}

function boxOf(el: Element): CardBox {
	const rect = el.getBoundingClientRect();
	return { x: rect.left, y: rect.top, width: rect.width, height: rect.height };
}

// The outermost card only: one drawn inside another card moves with its
// parent, and one inside an open dialog belongs to the dialog's own motion.
function isGridCard(el: Element): boolean {
	if (!el.isConnected) return false;
	if (el.closest(DIALOG_SELECTOR) !== null) return false;
	const parent = el.parentElement;
	return parent === null || parent.closest(CARD_SELECTOR) === null;
}

/** The boxes of every card on the grid, read before the layout class flips.
 *  No DOM (SSR) or no cards gives an empty map. */
export function captureCardBoxes(root?: ParentNode): Map<Element, CardBox> {
	const boxes = new Map<Element, CardBox>();
	const scope = root ?? (typeof document === 'undefined' ? undefined : document);
	if (!scope) return boxes;
	for (const el of Array.from(scope.querySelectorAll(CARD_SELECTOR))) {
		if (!isGridCard(el)) continue;
		boxes.set(el, boxOf(el));
	}
	return boxes;
}

/** Plays the morph from the captured boxes to wherever the cards now are.
 *  A card that moved or was resized between the capture and here keeps going from where it is,
 *  so a second press mid-flight is continuous. */
export async function playCardMorph(before: Map<Element, CardBox>): Promise<void> {
	if (typeof document === 'undefined' || before.size === 0 || reducedMotion()) return;
	// Svelte flushes the class change on the microtask, so the new boxes are
	// only readable once the tick has passed.
	await tick();
	for (const [el, from] of before) {
		if (!isGridCard(el)) continue;
		const transform = morphTransform(from, boxOf(el));
		if (transform === null) continue;
		el.animate([{ transform }, { transform: 'none' }], {
			duration: CARD_MORPH_MS,
			easing: CARD_MORPH_EASING
		});
	}
}
