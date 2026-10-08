/** The keyboard half of a native radio group: arrows move to the next choice, and Home and End
 *  jump to the ends. */
export function radioStep(index: number, length: number, key: string): number | null {
	if (length <= 0) return null;
	if (key === 'ArrowRight' || key === 'ArrowDown') return (index + 1) % length;
	if (key === 'ArrowLeft' || key === 'ArrowUp') return (index - 1 + length) % length;
	if (key === 'Home') return 0;
	if (key === 'End') return length - 1;
	return null;
}

/** What one radiogroup step is given: the key, the group it is in, the values those buttons
 *  stand for, and what to do with the one the step lands on. `flush` runs after the selection is
 *  made and before focus moves, so a group whose buttons are re-rendered stays one tab stop. */
export interface MoveRadio<T> {
	event: KeyboardEvent;
	root: HTMLElement;
	values: readonly T[];
	select: (value: T) => void;
	flush?: () => Promise<void>;
}

/** Applies that step inside one radiogroup. */
export async function moveRadio<T>(options: MoveRadio<T>): Promise<void> {
	const { event, root, values, select, flush = async () => {} } = options;
	if (event.altKey || event.metaKey || event.ctrlKey) return;
	const buttons = [...root.querySelectorAll<HTMLButtonElement>('[role="radio"]')];
	const current = buttons.findIndex((button) => button === document.activeElement);
	if (current < 0) return;
	const next = radioStep(current, values.length, event.key);
	if (next === null) return;
	const value = values[next];
	if (value === undefined) return;
	event.preventDefault();
	select(value);
	await flush();
	const updated = [...root.querySelectorAll<HTMLButtonElement>('[role="radio"]')];
	updated[next]?.focus();
}
