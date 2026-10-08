import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';

import { drawerEnter, drawerLeave, modalMotion, trackModalOrigin } from './modal-motion';

// The transition reads the viewport, the motion preference, and the pressed
// point, so the browser globals it asks for are stood in for. What is under
// test is the config each of those three produces.
let reducedMotion = false;
let innerWidth = 1280;
let activeElement: { rect: DOMRect | null } | null = null;

class FakeHTMLElement {}

// The source narrows the focused element with `instanceof HTMLElement`, so the
// stand-in installs that class and the element is a real instance of it.
const listeners = new Map<string, Array<(event: Event) => void>>();

function fire(name: string, props: Record<string, unknown>) {
	const event = Object.assign(new Event(name), props);
	for (const handler of [...(listeners.get(name) ?? [])]) handler(event);
}

function stubBrowser() {
	listeners.clear();
	reducedMotion = false;
	activeElement = null;

	vi.stubGlobal('HTMLElement', FakeHTMLElement);
	vi.stubGlobal('window', {
		innerWidth,
		matchMedia: (query: string) => ({ matches: reducedMotion && query.includes('reduce') }),
		addEventListener: (name: string, handler: (event: Event) => void) => {
			listeners.set(name, [...(listeners.get(name) ?? []), handler]);
		},
		removeEventListener: (name: string, handler: (event: Event) => void) => {
			listeners.set(
				name,
				(listeners.get(name) ?? []).filter((kept) => kept !== handler)
			);
		}
	});
	vi.stubGlobal('document', {
		get activeElement() {
			if (!activeElement) return null;
			return Object.assign(new FakeHTMLElement(), {
				getBoundingClientRect: () => activeElement!.rect
			});
		}
	});
}

function stubActiveElement(rect: DOMRect) {
	activeElement = { rect };
}

// A phone-width viewport: stubBrowser reads innerWidth when it runs, so the
// width is set before it is called again.
function narrow() {
	innerWidth = 390;
	stubBrowser();
}

/** A rectangle every member of, so a stand-in needs no assertion to be one. */
function rect(left: number, top: number, width: number, height: number): DOMRect {
	return {
		left,
		top,
		width,
		height,
		right: left + width,
		bottom: top + height,
		x: left,
		y: top,
		toJSON: () => ({})
	};
}

function node(value: DOMRect = rect(0, 0, 800, 600)): HTMLElement {
	return Object.assign(new FakeHTMLElement(), {
		style: {},
		getBoundingClientRect: () => value
	}) as unknown as HTMLElement;
}

beforeEach(() => {
	innerWidth = 1280;
	stubBrowser();
});

afterEach(() => {
	vi.unstubAllGlobals();
});

describe('modalMotion transition', () => {
	test('scales a modal in on a wide viewport', () => {
		const config = modalMotion(node());
		expect(config.duration).toBe(180);
		expect(config.css?.(0, 0)).toBe('opacity: 0; transform: scale(0.92)');
		expect(config.css?.(1, 0)).toBe('opacity: 1; transform: scale(1)');
	});

	test('slides a modal up as a drawer on a narrow viewport', () => {
		narrow();
		const config = modalMotion(node(), { startY: () => 40 });

		expect(config.duration).toBe(260);
		expect(config.css?.(0, 0)).toContain('translateY(calc(0px + 100%)');
		expect(config.css?.(1, 0)).toContain('translateY(calc(40px + 0%)');
	});

	test('starts a drawer at the top when no start position is named', () => {
		narrow();
		const config = modalMotion(node());
		expect(config.css?.(0, 0)).toContain('0px');
	});

	test('starts a drawer at the top rather than above it', () => {
		narrow();
		const config = modalMotion(node(), { startY: () => -20 });
		expect(config.css?.(1, 0)).toContain('0px');
	});

	test('fades rather than moves when reduced motion is requested', () => {
		reducedMotion = true;
		const config = modalMotion(node());

		expect(config.duration).toBe(140);
		expect(config.css?.(0.5, 0)).toBe('opacity: 0.5');
	});

	test('grows from the point the modal was opened at', () => {
		const stop = trackModalOrigin();
		fire('pointerdown', { clientX: 200, clientY: 100 });

		// The element the transition runs on is 800x600 at the origin, so a
		// press at (200, 100) is 200,100 inside it.
		const element = node();
		modalMotion(element);
		stop();

		expect(element.style.transformOrigin).toBe('200px 100px');
	});

	test('centres a modal opened from the keyboard', () => {
		const stop = trackModalOrigin();
		// A pointer is remembered first, then a keystroke clears it, because a
		// keyboard shortcut has no position to grow from.
		fire('pointerdown', { clientX: 200, clientY: 100 });
		fire('keydown', {});

		const element = node();
		modalMotion(element);
		stop();

		expect(element.style.transformOrigin).toBe('center');
	});

	test('forgets a pressed point once it is stale', () => {
		const stop = trackModalOrigin();
		fire('pointerdown', { clientX: 200, clientY: 100 });

		// Past the two seconds an origin is worth keeping for, the modal falls
		// back to its centre rather than growing from an old press.
		const realNow = Date.now;
		Date.now = () => realNow() + 5_000;
		const element = node();
		modalMotion(element);
		Date.now = realNow;
		stop();

		expect(element.style.transformOrigin).toBe('center');
	});

	test('grows from the focused element when a click carries no coordinates', () => {
		const stop = trackModalOrigin();
		// A keyboard-activated control reports detail 0, and its centre is
		// where the modal grows from.
		stubActiveElement(rect(40, 20, 100, 20));
		fire('click', { detail: 0 });

		const element = node();
		modalMotion(element);
		stop();

		expect(element.style.transformOrigin).toBe('90px 30px');
	});

	test('ignores a click that carries a pointer position', () => {
		const stop = trackModalOrigin();
		stubActiveElement(rect(40, 20, 100, 20));
		// A real mouse click has a detail count, and so has no position to
		// remember from the focused element.
		fire('click', { detail: 1 });

		const element = node();
		modalMotion(element);
		stop();

		expect(element.style.transformOrigin).toBe('center');
	});

	test('centres a modal whose bounds are not known yet', () => {
		const stop = trackModalOrigin();
		fire('pointerdown', { clientX: 200, clientY: 100 });

		// A modal measured before it has laid out has no origin to grow from.
		const element = node(rect(0, 0, 0, 0));
		modalMotion(element);
		stop();

		expect(element.style.transformOrigin).toBe('center');
	});
});

describe('trackModalOrigin', () => {
	test('listens once while at least one modal is tracking', () => {
		const first = trackModalOrigin();
		const second = trackModalOrigin();

		expect(listeners.get('pointerdown')).toHaveLength(1);

		// Releasing one leaves the other tracked, so the listeners stay.
		first();
		expect(listeners.get('pointerdown')).toHaveLength(1);

		second();
		expect(listeners.get('pointerdown')).toHaveLength(0);
	});

	test('stops listening once the last modal releases', () => {
		const stop = trackModalOrigin();
		stop();

		expect(listeners.get('pointerdown')).toHaveLength(0);
		expect(listeners.get('click')).toHaveLength(0);
		expect(listeners.get('keydown')).toHaveLength(0);
	});

	test('releases cleanly more than once', () => {
		const stop = trackModalOrigin();
		stop();
		expect(() => stop()).not.toThrow();
	});
});
describe('drawer transitions', () => {
	test('slides the drawer in from the left edge', () => {
		const config = drawerEnter(node());

		expect(config.duration).toBe(260);
		expect(config.css?.(0, 0)).toBe('transform: translateX(-100%)');
		expect(config.css?.(1, 0)).toBe('transform: translateX(0%)');
	});

	test('slides the drawer out a touch quicker than it came in', () => {
		const config = drawerLeave(node());

		expect(config.duration).toBe(220);
		expect(config.css?.(1, 0)).toContain('0px');
		expect(config.css?.(0, 0)).toContain('- 100%');
	});

	test('continues a drag-release outro from the finger offset', () => {
		const config = drawerLeave(node(), { startX: () => -48 });

		expect(config.css?.(1, 0)).toContain('-48px');
		expect(config.css?.(0, 0)).toContain('- 100%');
	});

	test('leaves from fully open rather than past it', () => {
		const config = drawerLeave(node(), { startX: () => 20 });

		expect(config.css?.(1, 0)).toContain('0px');
	});

	test('fades rather than slides when reduced motion is requested', () => {
		reducedMotion = true;

		for (const config of [drawerEnter(node()), drawerLeave(node())]) {
			expect(config.duration).toBe(140);
			expect(config.css?.(0.5, 0)).toBe('opacity: 0.5');
		}
	});
});
