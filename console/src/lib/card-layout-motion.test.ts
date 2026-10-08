import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';

import {
	CARD_MORPH_EASING,
	CARD_MORPH_EPSILON,
	CARD_MORPH_MS,
	captureCardBoxes,
	morphTransform,
	playCardMorph,
	reducedMotion
} from './card-layout-motion';

// The morph reads the viewport boxes of the cards and hands each one a Web
// Animation, so the browser globals it asks for are stood in for. What is
// under test is the shape of the capture, the transform, and the playback.

type Attrs = Record<string, string>;

interface FakeElement {
	isConnected: boolean;
	rect: { left: number; top: number; width: number; height: number };
	parentElement: FakeElement | null;
	attrs: Attrs;
	plays: Array<{ transform: string; duration: number; easing: string }>;
	animate(
		keyframes: Array<{ transform: string }>,
		options: { duration: number; easing: string }
	): void;
	closest(selector: string): FakeElement | null;
	getBoundingClientRect(): { left: number; top: number; width: number; height: number };
}

let reduced = false;
let cards: FakeElement[] = [];

function makeElement(attrs: Attrs = {}, parentElement: FakeElement | null = null): FakeElement {
	const element: FakeElement = {
		isConnected: true,
		rect: { left: 0, top: 0, width: 200, height: 100 },
		parentElement,
		attrs,
		plays: [],
		animate(keyframes, options) {
			element.plays.push({
				transform: keyframes[0].transform,
				duration: options.duration,
				easing: options.easing
			});
		},
		closest(selector: string) {
			const [name, quoted] = selector.slice(1, -1).split('=');
			const value = (quoted ?? '').replace(/^"|"$/g, '');
			let node: FakeElement | null = element;
			while (node) {
				if (node.attrs[name] === value) return node;
				node = node.parentElement;
			}
			return null;
		},
		getBoundingClientRect() {
			return element.rect;
		}
	};
	return element;
}

// The morph only calls element methods the fake installs, so the stand-in
// stands in for the DOM type at the module boundary.
function asElement(el: FakeElement): Element {
	return el as unknown as Element;
}

// The fake root hands back exactly the cards the test installed.
function makeRoot(): ParentNode {
	return {
		querySelectorAll: () => cards.map(asElement)
	} as unknown as ParentNode;
}

function stubBrowser() {
	vi.stubGlobal('window', {
		matchMedia: (query: string) => ({ matches: reduced && query.includes('reduce') })
	});
	vi.stubGlobal('document', {
		querySelectorAll: () => cards.map(asElement)
	});
}

beforeEach(() => {
	reduced = false;
	cards = [];
	stubBrowser();
});

afterEach(() => {
	vi.unstubAllGlobals();
});

describe('morphTransform', () => {
	test('translates alone when only the position changed', () => {
		expect(
			morphTransform(
				{ x: 0, y: 0, width: 200, height: 100 },
				{ x: 30, y: -12, width: 200, height: 100 }
			)
		).toBe('translate(30px, -12px) scale(1, 1)');
	});

	test('scales from the width and height ratio', () => {
		expect(
			morphTransform(
				{ x: 0, y: 0, width: 200, height: 100 },
				{ x: 0, y: 0, width: 300, height: 50 }
			)
		).toBe('translate(0px, 0px) scale(1.5, 0.5)');
	});

	test('returns nothing when every delta is under the epsilon', () => {
		const near = CARD_MORPH_EPSILON / 2;
		expect(
			morphTransform(
				{ x: 10, y: 10, width: 200, height: 100 },
				{ x: 10 + near, y: 10 - near, width: 200 + near, height: 100 - near }
			)
		).toBeNull();
	});

	test('takes scale one for a card that never had a size', () => {
		expect(
			morphTransform({ x: 0, y: 0, width: 0, height: 0 }, { x: 4, y: 0, width: 0, height: 0 })
		).toBe('translate(4px, 0px) scale(1, 1)');
	});
});

describe('reducedMotion', () => {
	test('reads the motion preference', () => {
		expect(reducedMotion()).toBe(false);
		reduced = true;
		expect(reducedMotion()).toBe(true);
	});
});

describe('captureCardBoxes', () => {
	test('reads one box per card', () => {
		const card = makeElement();
		card.rect = { left: 12, top: 8, width: 240, height: 120 };
		cards = [card];

		const boxes = captureCardBoxes(makeRoot());

		expect(boxes.get(asElement(card))).toEqual({ x: 12, y: 8, width: 240, height: 120 });
	});

	test('skips a card nested inside another card', () => {
		const outer = makeElement({ 'data-slot': 'card' });
		const inner = makeElement({ 'data-slot': 'card' }, outer);
		cards = [outer, inner];

		expect([...captureCardBoxes(makeRoot()).keys()]).toEqual([asElement(outer)]);
	});

	test('skips a card inside a dialog and a disconnected one', () => {
		const dialog = makeElement({ role: 'dialog' });
		const inside = makeElement({ 'data-slot': 'card' }, dialog);
		const gone = makeElement({ 'data-slot': 'card' });
		gone.isConnected = false;
		cards = [inside, gone];

		expect(captureCardBoxes(makeRoot()).size).toBe(0);
	});
});

describe('playCardMorph', () => {
	const oldBox = { x: 0, y: 0, width: 200, height: 100 };

	test('animates each card from its old box to the new one', async () => {
		const card = makeElement();
		const before = new Map([[asElement(card), oldBox]]);
		card.rect = { left: 40, top: 0, width: 200, height: 100 };
		cards = [card];

		await playCardMorph(before);

		expect(card.plays).toEqual([
			{
				transform: 'translate(40px, 0px) scale(1, 1)',
				duration: CARD_MORPH_MS,
				easing: CARD_MORPH_EASING
			}
		]);
	});

	test('starts no animation under reduced motion', async () => {
		reduced = true;
		const card = makeElement();
		const before = new Map([[asElement(card), oldBox]]);
		card.rect = { left: 40, top: 0, width: 200, height: 100 };
		cards = [card];

		await playCardMorph(before);

		expect(card.plays).toEqual([]);
	});

	test('skips a card that was removed between capture and play', async () => {
		const card = makeElement();
		const before = new Map([[asElement(card), oldBox]]);
		card.rect = { left: 40, top: 0, width: 200, height: 100 };
		card.isConnected = false;
		cards = [card];

		await playCardMorph(before);

		expect(card.plays).toEqual([]);
	});

	test('skips a card that did not move', async () => {
		const card = makeElement();
		const before = new Map([[asElement(card), oldBox]]);
		cards = [card];

		await playCardMorph(before);

		expect(card.plays).toEqual([]);
	});
});
