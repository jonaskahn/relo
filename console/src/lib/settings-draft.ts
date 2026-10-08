// settings-draft.ts holds the draft bookkeeping every explicit-save settings
// card shares: the copy an operator edits, and the stored copy it is compared
// against to know there is something to save.

/** A card's group arrives as reactive state, and structuredClone refuses to clone a state proxy;
 *  a JSON round-trip copies the values out of it. The shape is the caller's own, since it round-
 *  tripped the same value a moment ago. */
export function cloneDraft<T>(value: T): T {
	return JSON.parse(JSON.stringify(value));
}

/** Reports whether an edited form still differs from what is saved. */
export function draftDirty<T>(draft: T, saved: T): boolean {
	return JSON.stringify(draft) !== JSON.stringify(saved);
}
