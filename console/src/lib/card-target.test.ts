import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';

import { isCardOpenClick } from './card-target';

// The guard reads the click's target and the document's selection, so both
// are stood in for. A control is any element whose closest control ancestor
// is itself or above it.

let collapsed = true;

function stubBrowser() {
	vi.stubGlobal('document', {
		getSelection: () => ({ isCollapsed: collapsed })
	});
}

function click(target: unknown, over: Partial<MouseEvent> = {}): MouseEvent {
	return {
		button: 0,
		defaultPrevented: false,
		target,
		...over
	} as MouseEvent;
}

// A stand-in element: `closest` walks up from the target in the DOM, so the
// fake answers whether the target itself is (or is inside) a control.
function element(control = false): Element {
	const node: { closest?: () => unknown } = {};
	node.closest = () => (control ? node : null);
	return node as unknown as Element;
}

beforeEach(() => {
	collapsed = true;
	stubBrowser();
});

afterEach(() => {
	vi.unstubAllGlobals();
});

describe('isCardOpenClick', () => {
	test('opens the card for a plain click on its content', () => {
		expect(isCardOpenClick(click(element()))).toBe(true);
	});

	test('leaves a click that began on a control to the control', () => {
		expect(isCardOpenClick(click(element(true)))).toBe(false);
	});

	test('leaves a click that ended in a text selection alone', () => {
		collapsed = false;
		expect(isCardOpenClick(click(element()))).toBe(false);
	});

	test('ignores a click that is not the primary button', () => {
		expect(isCardOpenClick(click(element(), { button: 2 }))).toBe(false);
	});

	test('ignores a click a handler has already taken', () => {
		expect(isCardOpenClick(click(element(), { defaultPrevented: true }))).toBe(false);
	});

	test('opens the card for a click on a target that is not an element', () => {
		expect(isCardOpenClick(click(null))).toBe(true);
	});
});
