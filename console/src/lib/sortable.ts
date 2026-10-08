/** The pure reorder a drag or an arrow key commits: the item at from lands at to and every
 *  neighbour shifts by one. */
export function moveMemberTo<T>(list: T[], from: number, to: number): T[] {
	if (from === to) return list;
	if (from < 0 || to < 0 || from >= list.length || to >= list.length) return list;
	const next = [...list];
	const [item] = next.splice(from, 1);
	next.splice(to, 0, item);
	return next;
}

/** The contract a drag-reorder list hands the attachment. */
export interface SortableOptions {
	// handle is the selector, inside the sortable element, that starts a drag.
	handle: string;
	// onMove commits one reorder. Keyboard reordering goes through the same
	// commit, so the list state stays in one place.
	onMove: (from: number, to: number) => void;
	disabled?: () => boolean;
}

/** Builds the attachment that gives a list pointer-based reordering.
 *  The row follows the finger or mouse one to one from where it was grabbed, releases past a
 *  third of a row height commit, and Escape cancels.
 *  The options are read when a gesture starts, so a list that re-attaches mid-drag never loses
 *  the listeners the drag already took out. */
export function sortable(options: SortableOptions): (node: HTMLElement) => () => void {
	return (node) => {
		function onPointerDown(event: PointerEvent) {
			if (options.disabled?.()) return;
			const target = event.target instanceof Element ? event.target : null;
			const handle = target?.closest(options.handle);
			if (!handle) return;
			const row = handle.closest<HTMLElement>('[data-sortable-row]');
			if (!row) return;
			event.preventDefault();
			const siblings = Array.from(node.querySelectorAll<HTMLElement>('[data-sortable-row]'));
			const from = siblings.indexOf(row);
			if (from < 0) return;
			const startY = event.clientY;
			let moved = false;

			const rect = row.getBoundingClientRect();
			const grabOffset = startY - rect.top;
			row.style.zIndex = '10';
			row.style.position = 'relative';
			row.style.touchAction = 'none';

			const move = (moveEvent: PointerEvent) => {
				const dy = moveEvent.clientY - startY;
				if (!moved && Math.abs(dy) < 6) return;
				moved = true;
				row.style.transform = 'translateY(' + dy + 'px)';
				// The row the pointer is over takes the slot the dragged row would
				// land in if the gesture ended here.
				const ghostTop = moveEvent.clientY - grabOffset;
				let to = from;
				siblings.forEach((sibling, index) => {
					if (index === from) return;
					const mid =
						sibling.getBoundingClientRect().top + sibling.getBoundingClientRect().height / 2;
					if (index < from && ghostTop < mid) to = Math.min(to, index);
					if (index > from && ghostTop + row.offsetHeight > mid) to = Math.max(to, index);
				});
				siblings.forEach((sibling, index) => {
					if (index === from) return;
					const shift = index > from && index <= to ? -1 : index < from && index >= to ? 1 : 0;
					sibling.style.transform =
						shift === 0 ? '' : 'translateY(' + shift * row.offsetHeight + 'px)';
					sibling.style.transition = 'transform 150ms';
				});
				row.dataset.sortableTarget = String(to);
			};

			const finish = () => {
				window.removeEventListener('pointermove', move);
				window.removeEventListener('pointerup', finish);
				window.removeEventListener('pointercancel', cancel);
				row.style.zIndex = '';
				row.style.position = '';
				row.style.transform = '';
				siblings.forEach((sibling) => {
					sibling.style.transform = '';
					sibling.style.transition = '';
				});
				if (!moved) return;
				const to = Number(row.dataset.sortableTarget ?? from);
				delete row.dataset.sortableTarget;
				if (to !== from) options.onMove(from, to);
			};
			const cancel = () => {
				window.removeEventListener('pointermove', move);
				window.removeEventListener('pointerup', finish);
				window.removeEventListener('pointercancel', cancel);
				row.style.zIndex = '';
				row.style.position = '';
				row.style.transform = '';
				siblings.forEach((sibling) => {
					sibling.style.transform = '';
					sibling.style.transition = '';
				});
				delete row.dataset.sortableTarget;
			};

			window.addEventListener('pointermove', move);
			window.addEventListener('pointerup', finish);
			window.addEventListener('pointercancel', cancel);
		}

		node.addEventListener('pointerdown', onPointerDown);
		return () => node.removeEventListener('pointerdown', onPointerDown);
	};
}
