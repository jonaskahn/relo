import { parseContextInput } from './context-window';

// The clone dialog's rules, kept pure so the dialog and the tests read one
// implementation.

/** Names a clone the way an operator would: <id>-copy, then -copy-2, and so on until the id is
 *  free. */
export function suggestedCloneId(baseId: string, taken: Iterable<string>): string {
	const used = new Set(taken);
	const first = baseId + '-copy';
	if (!used.has(first)) return first;
	for (let n = 2; n < 1000; n++) {
		const candidate = first + '-' + n;
		if (!used.has(candidate)) return candidate;
	}
	return first;
}

/** Why a cloned model id cannot be stored. */
export type CloneIdProblem = 'empty' | 'whitespace' | 'tooLong' | 'taken';

/** Reports why an identifier cannot be used, or null when it can.
 *  The server's conflict answer is final, so this is only a head start. */
export function validateCloneId(id: string, taken: Iterable<string>): CloneIdProblem | null {
	const trimmed = id.trim();
	if (trimmed === '') return 'empty';
	if (/\s/.test(trimmed)) return 'whitespace';
	if (trimmed.length > 256) return 'tooLong';
	if (new Set(taken).has(trimmed)) return 'taken';
	return null;
}

/** What the dialog collects, with prices in dollars per million. */
export interface CloneForm {
	upstreamId: string;
	name: string;
	contextWindow: string;
	maxOutput: string;
	inputPrice: string;
	outputPrice: string;
}

/** What the model being copied already states. */
export interface CloneSource {
	upstreamId: string;
	name: string;
	contextWindow: number | null;
	maxOutput: number | null;
	inputPrice: number | null;
	outputPrice: number | null;
}

/** The request body: only the fields the operator changed. */
export interface ClonePayload {
	upstream_model_id?: string;
	override?: {
		name?: string;
		context_window?: number;
		max_output?: number;
		prices?: { input?: number; output?: number };
	};
}

/** Sends only what the operator changed, so a clone keeps the source's other details instead of
 *  blanking them. */
export function clonePayload(form: CloneForm, source: CloneSource): ClonePayload {
	const payload: ClonePayload = {};
	const upstream = form.upstreamId.trim();
	if (upstream !== '' && upstream !== source.upstreamId) payload.upstream_model_id = upstream;

	const override: NonNullable<ClonePayload['override']> = {};
	const name = form.name.trim();
	if (name !== '' && name !== source.name) override.name = name;

	const context = parseContextInput(form.contextWindow);
	if (context.ok && context.value !== source.contextWindow) override.context_window = context.value;

	const maxOutput = parseCount(form.maxOutput);
	if (maxOutput !== null && maxOutput !== source.maxOutput) override.max_output = maxOutput;

	const prices = priceDiff(form, source);
	if (prices) override.prices = prices;

	if (Object.keys(override).length > 0) payload.override = override;
	return payload;
}

function parseCount(raw: string): number | null {
	const text = raw.trim().replace(/[,\s]/g, '');
	if (text === '') return null;
	const value = Number(text);
	return Number.isFinite(value) && value > 0 ? Math.round(value) : null;
}

function priceDiff(
	form: CloneForm,
	source: CloneSource
): { input?: number; output?: number } | null {
	const diff: { input?: number; output?: number } = {};
	const input = parsePrice(form.inputPrice);
	if (input !== null && input !== source.inputPrice) diff.input = input;
	const output = parsePrice(form.outputPrice);
	if (output !== null && output !== source.outputPrice) diff.output = output;
	return Object.keys(diff).length > 0 ? diff : null;
}

/** Reads dollars per million tokens and converts to the integer micros the catalog stores. */
export function parsePrice(raw: string): number | null {
	const text = raw.trim().replace(/\$/g, '');
	if (text === '') return null;
	const value = Number(text);
	if (!Number.isFinite(value) || value < 0) return null;
	return Math.round(value * 1_000_000);
}

/** Writes stored micros back as dollars per million tokens. */
export function formatPrice(micros: number | null | undefined): string {
	if (micros === null || micros === undefined) return '';
	return String(micros / 1_000_000);
}
