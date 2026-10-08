import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { moveMemberTo, sortable, type SortableOptions } from './sortable';

describe('moveMemberTo', () => {
	it('puts the item at from at to and shifts the rest by one', () => {
		expect(moveMemberTo(['a', 'b', 'c', 'd'], 0, 2)).toEqual(['b', 'c', 'a', 'd']);
		expect(moveMemberTo(['a', 'b', 'c', 'd'], 3, 0)).toEqual(['d', 'a', 'b', 'c']);
		expect(moveMemberTo(['a', 'b', 'c', 'd'], 1, 2)).toEqual(['a', 'c', 'b', 'd']);
	});

	it('leaves the list alone for a move that changes nothing', () => {
		const list = ['a', 'b', 'c'];
		expect(moveMemberTo(list, 1, 1)).toBe(list);
	});

	it('leaves the list alone for an index the list does not have', () => {
		const list = ['a', 'b', 'c'];
		expect(moveMemberTo(list, -1, 0)).toBe(list);
		expect(moveMemberTo(list, 0, -1)).toBe(list);
		expect(moveMemberTo(list, 3, 0)).toBe(list);
		expect(moveMemberTo(list, 0, 3)).toBe(list);
	});

	it('leaves the caller list untouched', () => {
		const list = ['a', 'b', 'c'];
		moveMemberTo(list, 0, 2);
		expect(list).toEqual(['a', 'b', 'c']);
	});
});

// The attachment drives a pointer drag against real rows. These stand in for them,
// because what is under test is the reorder the gesture commits rather than a
// DOM: a row is a position, a height, and the styles the action writes.
const ROW_HEIGHT = 50;

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

interface Row {
	element: HTMLElement;
	top: number;
}

// The attachment narrows a pointer target with `instanceof Element`, so the rows
// and the targets below are instances of a stand-in Element.
class FakeElement {}

beforeEach(() => {
	vi.stubGlobal('Element', FakeElement);
});

afterEach(() => {
	vi.unstubAllGlobals();
});

function makeRow(top: number): Row {
	const element = Object.assign(new FakeElement(), {
		style: {},
		dataset: {},
		getBoundingClientRect: () => rect(0, top, 0, ROW_HEIGHT)
	});
	Object.defineProperty(element, 'offsetHeight', { value: ROW_HEIGHT, configurable: true });
	return { element: element as unknown as HTMLElement, top };
}

interface Drag {
	cleanup: () => void;
	press(index: number, clientY: number): void;
	drag(clientY: number): void;
	release(): void;
	cancel(): void;
}

function mountDrag(count: number, options: Partial<SortableOptions> = {}, rows?: Row[]): Drag {
	rows ??= Array.from({ length: count }, (_, index) => makeRow(index * ROW_HEIGHT));

	const windowHandlers = new Map<string, Array<(event: unknown) => void>>();
	vi.stubGlobal('window', {
		addEventListener: (name: string, handler: (event: unknown) => void) => {
			windowHandlers.set(name, [...(windowHandlers.get(name) ?? []), handler]);
		},
		removeEventListener: (name: string, handler: (event: unknown) => void) => {
			windowHandlers.set(
				name,
				(windowHandlers.get(name) ?? []).filter((kept) => kept !== handler)
			);
		}
	});

	let press: ((event: unknown) => void) | undefined;
	const node = {
		querySelectorAll: () => rows.map((row) => row.element),
		addEventListener: (name: string, handler: (event: unknown) => void) => {
			if (name === 'pointerdown') press = handler;
		},
		// A torn-down attachment stops listening, so a press after it reaches
		// nothing at all.
		removeEventListener: (name: string) => {
			if (name === 'pointerdown') press = undefined;
		}
	} as unknown as HTMLElement;

	const cleanup = sortable({ handle: '[data-handle]', onMove: vi.fn(), ...options })(node);

	const fire = (name: string, event: unknown) => {
		for (const handler of windowHandlers.get(name) ?? []) handler(event);
	};

	return {
		cleanup,
		press(index, clientY) {
			// A press on the handle of a row. The event target resolves to the
			// handle, and the handle in turn resolves to the row it belongs to.
			const handle = Object.assign(new FakeElement(), {
				closest: (selector: string) =>
					selector === '[data-sortable-row]' ? rows[index].element : null
			});
			const target = Object.assign(new FakeElement(), {
				closest: (selector: string) => (selector === '[data-handle]' ? handle : null)
			});
			press?.({ target, clientY, preventDefault: () => undefined });
		},
		drag: (clientY) => fire('pointermove', { clientY }),
		release: () => fire('pointerup', {}),
		cancel: () => fire('pointercancel', {})
	};
}

describe('sortable', () => {
	it('commits a drag past the middle of the next row', () => {
		const onMove = vi.fn();
		const drag = mountDrag(3, { onMove });

		// Grab the first row at its top, so the grab offset is nothing and the
		// row's top follows the pointer exactly. The three rows are 50px tall
		// with their midpoints at 25, 75, and 125. At a pointer of 60 the
		// dragged row spans 60-110, which is past the second row's midpoint
		// but short of the third's: the row lands at index 1.
		drag.press(0, 0);
		drag.drag(60);
		drag.release();

		expect(onMove).toHaveBeenCalledExactlyOnceWith(0, 1);
	});

	it('clears every style the drag wrote, so the list is left as it was', () => {
		const rows = makeRow(0);
		const drag = mountDrag(3, {}, [rows, makeRow(50), makeRow(100)]);

		drag.press(0, 0);
		drag.drag(60);
		expect(rows.element.style.transform).not.toBe('');

		drag.release();
		// The attachment clears each style by assigning the empty string.
		expect(rows.element.style.zIndex ?? '').toBe('');
		expect(rows.element.style.position ?? '').toBe('');
		expect(rows.element.style.transform ?? '').toBe('');
	});

	it('does not commit a press and a release in the same place', () => {
		const onMove = vi.fn();
		const drag = mountDrag(3, { onMove });

		drag.press(0, 10);
		drag.release();

		expect(onMove).not.toHaveBeenCalled();
	});

	it('does not commit a drag that never left its own row', () => {
		const onMove = vi.fn();
		const drag = mountDrag(3, { onMove });

		// Under six pixels of movement the gesture is read as a click, not a
		// drag, so nothing is reordered.
		drag.press(0, 25);
		drag.drag(28);
		drag.release();

		expect(onMove).not.toHaveBeenCalled();
	});

	it('commits nothing when the pointer is cancelled', () => {
		const onMove = vi.fn();
		const drag = mountDrag(3, { onMove });

		drag.press(0, 0);
		drag.drag(80);
		drag.cancel();

		expect(onMove).not.toHaveBeenCalled();
	});

	it('ignores a press that did not land on a handle', () => {
		const onMove = vi.fn();
		const rows = Array.from({ length: 3 }, (_, index) => makeRow(index * ROW_HEIGHT));
		vi.stubGlobal('window', { addEventListener: vi.fn(), removeEventListener: vi.fn() });

		let press: ((event: unknown) => void) | undefined;
		const node = {
			querySelectorAll: () => rows.map((row) => row.element),
			addEventListener: (name: string, handler: (event: unknown) => void) => {
				if (name === 'pointerdown') press = handler;
			},
			// A torn-down attachment stops listening, so a press after it reaches
			// nothing at all.
			removeEventListener: (name: string) => {
				if (name === 'pointerdown') press = undefined;
			}
		} as unknown as HTMLElement;
		sortable({ handle: '[data-handle]', onMove })(node);

		// A target that is not an element has no handle to find.
		press?.({ target: 'not an element', clientY: 0, preventDefault: () => undefined });

		expect(onMove).not.toHaveBeenCalled();
	});

	it('does nothing while the list is disabled', () => {
		const onMove = vi.fn();
		const drag = mountDrag(3, { onMove, disabled: () => true });

		drag.press(0, 0);
		drag.drag(80);
		drag.release();

		expect(onMove).not.toHaveBeenCalled();
	});

	it('commits through the options it was attached with', () => {
		const before = vi.fn();
		const after = vi.fn();
		const first = mountDrag(3, { onMove: before });
		first.cleanup();
		// An attachment carries the options it was built with, so a list
		// picks up new ones by attaching again rather than by updating.
		const second = mountDrag(3, { onMove: after });

		second.press(0, 0);
		second.drag(60);
		second.release();

		expect(before).not.toHaveBeenCalled();
		expect(after).toHaveBeenCalledExactlyOnceWith(0, 1);
	});

	it('stops listening to the list once it is torn down', () => {
		const onMove = vi.fn();
		const drag = mountDrag(3, { onMove });
		drag.cleanup();
		drag.press(0, 0);
		drag.drag(60);
		drag.release();
		expect(onMove).not.toHaveBeenCalled();
	});
});
