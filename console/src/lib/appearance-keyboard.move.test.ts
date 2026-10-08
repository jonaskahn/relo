import { afterEach, describe, expect, it, vi } from 'vitest';

import { moveRadio } from './appearance-keyboard';

// moveRadio drives a delegated radio group: it reads the focused button out of
// the group, applies the step, flushes, and moves focus. The group is stood in
// for because what is under test is the selection and focus it commits.
afterEach(() => {
	vi.unstubAllGlobals();
});

interface RadioButton {
	focused: boolean;
	focus(): void;
}

interface Group {
	root: HTMLElement;
	buttons: RadioButton[];
	/** The button the event carries focus, or null when focus is elsewhere. */
	focusedIndex(): number;
	focus(index: number): void;
	press(
		key: string,
		modifiers?: { altKey?: boolean; metaKey?: boolean; ctrlKey?: boolean }
	): KeyboardEvent;
}

/** makeGroup builds a group of count buttons and puts focus on the first. */
function makeGroup(count: number, focusIndex: number | null = 0): Group {
	const buttons: RadioButton[] = Array.from({ length: count }, () => ({
		focused: false,
		focus() {}
	}));

	const nodes = buttons.map((button, index) => {
		button.focus = () => {
			for (const candidate of buttons) candidate.focused = false;
			button.focused = true;
		};
		void index;
		return { role: 'radio', focus: () => button.focus() } as unknown as HTMLElement;
	});

	let active = focusIndex === null ? -1 : focusIndex;
	if (focusIndex !== null) buttons[focusIndex].focused = true;

	const root = { querySelectorAll: () => nodes } as unknown as HTMLElement;
	vi.stubGlobal('document', {
		get activeElement() {
			// A focus outside the group is reported as an element the group does
			// not hold, which is what indexOf answers -1 to.
			return active < 0 ? { role: 'textbox' } : nodes[active];
		}
	});

	return {
		root,
		buttons,
		focusedIndex: () => buttons.findIndex((button) => button.focused),
		focus: (index) => {
			active = index;
			buttons[index].focus();
		},
		press: (key, modifiers = {}) =>
			({
				key,
				altKey: false,
				metaKey: false,
				ctrlKey: false,
				shiftKey: false,
				preventDefault: () => undefined,
				...modifiers
			}) as unknown as KeyboardEvent
	};
}

describe('moveRadio', () => {
	it('selects the next choice and moves focus to it', async () => {
		const group = makeGroup(3);
		const select = vi.fn();

		await moveRadio({
			event: group.press('ArrowRight'),
			root: group.root,
			values: ['used', 'remaining', 'none'],
			select: select
		});

		expect(select).toHaveBeenCalledExactlyOnceWith('remaining');
		expect(group.focusedIndex()).toBe(1);
	});

	it('wraps past the end of the group', async () => {
		const group = makeGroup(3);
		group.focus(2);
		const select = vi.fn();

		await moveRadio({
			event: group.press('ArrowRight'),
			root: group.root,
			values: ['a', 'b', 'c'],
			select: select
		});

		expect(select).toHaveBeenCalledExactlyOnceWith('a');
	});

	it('waits for the flush before focus moves', async () => {
		// The group stays one tab stop only if the new selection is committed
		// before focus lands on the button that carries it.
		const order: string[] = [];
		const group = makeGroup(2);

		await moveRadio({
			event: group.press('ArrowRight'),
			root: group.root,
			values: ['a', 'b'],
			select: () => order.push('select'),
			flush: async () => {
				order.push('flush');
			}
		});

		expect(order).toEqual(['select', 'flush']);
		expect(group.focusedIndex()).toBe(1);
	});

	it('moves to the first and last choice', async () => {
		const group = makeGroup(3);
		group.focus(1);
		const select = vi.fn();

		await moveRadio({
			event: group.press('Home'),
			root: group.root,
			values: ['a', 'b', 'c'],
			select: select
		});
		expect(select).toHaveBeenCalledExactlyOnceWith('a');

		select.mockClear();
		await moveRadio({
			event: group.press('End'),
			root: group.root,
			values: ['a', 'b', 'c'],
			select: select
		});
		expect(select).toHaveBeenCalledExactlyOnceWith('c');
	});

	it('leaves the group alone for a key it does not handle', async () => {
		const group = makeGroup(3);
		const select = vi.fn();

		await moveRadio({
			event: group.press('Enter'),
			root: group.root,
			values: ['a', 'b', 'c'],
			select: select
		});

		expect(select).not.toHaveBeenCalled();
		expect(group.focusedIndex()).toBe(0);
	});

	it('leaves the group alone when a modifier is held', async () => {
		// Alt and the platform keys belong to the browser and the OS, so a
		// group must not swallow them.
		for (const modifier of ['altKey', 'metaKey', 'ctrlKey'] as const) {
			const group = makeGroup(3);
			const select = vi.fn();

			await moveRadio({
				event: group.press('ArrowRight', { [modifier]: true }),
				root: group.root,
				values: ['a', 'b', 'c'],
				select: select
			});

			expect(select, modifier).not.toHaveBeenCalled();
		}
	});

	it('does nothing when focus is outside the group', async () => {
		// Focus somewhere else on the page: there is no index to step from.
		const group = makeGroup(3, null);
		const select = vi.fn();

		await moveRadio({
			event: group.press('ArrowRight'),
			root: group.root,
			values: ['a', 'b', 'c'],
			select: select
		});

		expect(select).not.toHaveBeenCalled();
	});

	it('does nothing when the group has no choices to move through', async () => {
		// A group whose values have not loaded yet, or a filter that removed
		// every option: there is no step to take.
		const group = makeGroup(3);
		const select = vi.fn();

		await moveRadio({
			event: group.press('ArrowRight'),
			root: group.root,
			values: [],
			select: select
		});

		expect(select).not.toHaveBeenCalled();
		expect(group.focusedIndex()).toBe(0);
	});

	it('steps by the values rather than by the buttons on screen', async () => {
		// Fewer values than buttons, which a stale render can leave behind:
		// the step follows the choices, so it never lands past the last one.
		const group = makeGroup(3);
		group.focus(2);
		const select = vi.fn();

		await moveRadio({
			event: group.press('ArrowRight'),
			root: group.root,
			values: ['only'],
			select: select
		});

		expect(select).toHaveBeenCalledExactlyOnceWith('only');
	});
});
