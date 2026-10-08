import type { AccountContextModel, ContextLayers } from '$lib/types';

// The context picker's rules, kept pure so the popover, the clone dialog and
// the tests all read one implementation.

/** The sizes an operator picks from, in decimal units:
 *  256K means 256,000 tokens rather than 262,144.
 *  The list is the one every surface offers, from a model row's quick chips to the bulk "context
 *  for all" menu, and the sizes a detected window snaps to. "Default" is not a preset; it clears
 *  the override so the provider and models.dev layers decide again. */
export const CONTEXT_PRESETS: readonly number[] = [
	128_000, 200_000, 256_000, 300_000, 400_000, 512_000, 800_000, 1_000_000
];

/** The smallest window a model may state. */
export const MIN_CONTEXT_WINDOW = 1_000;
/** The largest window a model may state. */
export const MAX_CONTEXT_WINDOW = 100_000_000;

/** Writes a token count the way an operator reads it: millions keep one useful decimal, and
 *  smaller round numbers keep their K unit. */
export function contextLabel(value: number | null | undefined): string {
	if (value === null || value === undefined) return '';
	if (value >= 1_000_000) return trimNumber(Math.round((value / 1_000_000) * 10) / 10) + 'M';
	if (value >= 1_000 && value % 1_000 === 0) return trimNumber(value / 1_000) + 'K';
	return value.toLocaleString();
}

function trimNumber(value: number): string {
	return Number.isInteger(value) ? String(value) : String(value);
}

/** Lands a count on the nearest thousand, which is the unit contextLabel writes as K or M.
 *  262,144 becomes 262,000. 1,456,789 becomes 1,457,000. */
export function roundTokenCount(value: number): number {
	return Math.round(value / 1_000) * 1_000;
}

/** What an operator's typing resolved to, or why it did not. */
export type ContextParse =
	{ ok: true; value: number } | { ok: false; reason: 'empty' | 'unreadable' | 'range' };

/** Reads what an operator typed. A bare number is tokens, and a K or M suffix is decimal, so
 *  300k is 300,000 and 1.5M is 1,500,000. */
export function parseContextInput(raw: string): ContextParse {
	const text = raw
		.trim()
		.toLowerCase()
		.replace(/[\s_,]/g, '');
	if (text === '') return { ok: false, reason: 'empty' };
	const match = /^(\d+(?:\.\d+)?)([km])?$/.exec(text);
	if (!match) return { ok: false, reason: 'unreadable' };
	const amount = Number(match[1]);
	const scale = match[2] === 'm' ? 1_000_000 : match[2] === 'k' ? 1_000 : 1;
	const value = Math.round(amount * scale);
	if (!Number.isFinite(value) || value < MIN_CONTEXT_WINDOW || value > MAX_CONTEXT_WINDOW) {
		return { ok: false, reason: 'range' };
	}
	return { ok: true, value };
}

/** Names the layer an effective context window came from. */
export const CONTEXT_SOURCE_KEY: Record<string, string> = {
	override: 'ui.pages.providersPage.context.sourceYou',
	provider: 'ui.pages.providersPage.context.sourceProvider',
	modelsdev: 'ui.pages.providersPage.context.sourceModelsDev',
	'': 'ui.pages.providersPage.context.sourceNone'
};

/** Reports whether an operator set the context window by hand, which is what the cell marks and
 *  what Default clears. */
export function hasContextOverride(layers: ContextLayers | undefined): boolean {
	return layers !== undefined && layers.override !== null;
}

/** The one character a model row prints beside its context value: nothing while the layers
 *  decide, an asterisk for the operator's own override, and an exclamation when that override is
 *  above the maximum input the model states — a size the daemon stores rather than refuses. */
export type ContextMark = '' | '*' | '!';

/** Reads a window size as the short mark a card shows. */
export function contextMark(
	layers: ContextLayers | undefined,
	maxInput: number | null | undefined
): ContextMark {
	const override = layers?.override ?? null;
	if (override === null) return '';
	if (maxInput !== null && maxInput !== undefined && override > maxInput) return '!';
	return '*';
}

/** Names the models a batch write sizes for one account: the models that account is known to
 *  serve, and its whole list when the daemon knows of none, since an override on a model the
 *  account cannot reach would never apply. */
export function contextTargets(models: AccountContextModel[]): string[] {
	const listed = models.filter((model) => model.listed).map((model) => model.model_id);
	return listed.length > 0 ? listed : models.map((model) => model.model_id);
}
