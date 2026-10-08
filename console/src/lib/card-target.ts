// The whole card is one navigation target, but the controls inside it keep
// their own jobs: a click that began on a control, or that ended in a text
// selection, is not a request to open the card.
const CONTROL =
	'button, a, input, select, textarea, summary, [role="button"], [role="radio"], [role="switch"], [role="checkbox"]';

// Only an element carries a control ancestor: a text node or the document
// itself has no `closest` to walk, so a click on one reaches no control.
function canWalk(
	target: EventTarget | null
): target is EventTarget & { closest(selectors: string): Element } {
	return (
		typeof target === 'object' &&
		target !== null &&
		'closest' in target &&
		typeof target.closest === 'function'
	);
}

/** Reports whether a click on a card is one that opens it, rather than a click on a control
 *  inside it or the tail of a text selection. */
export function isCardOpenClick(event: MouseEvent): boolean {
	if (event.button !== 0 || event.defaultPrevented) return false;
	const target = event.target;
	if (canWalk(target) && target.closest(CONTROL)) return false;
	return document.getSelection()?.isCollapsed !== false;
}
