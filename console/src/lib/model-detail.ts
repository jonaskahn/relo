import type { DetailLayer, Model, ModelDetails } from '$lib/types';

/** A layer that is silent about every value, which is what a layer nobody wrote holds. */
export function emptyLayer(): DetailLayer {
	return {
		name: null,
		description: null,
		family: null,
		category: null,
		context_window: null,
		max_input: null,
		max_output: null,
		tools: null,
		reasoning: null,
		vision: null,
		status: null,
		release_date: null
	};
}

/** Shapes the values a list row already carries into the layer shape the expanded panel reads.
 *  A list carries no detail layers, so the panel renders from this until the single-model read
 *  lands — and keeps rendering from it when that read is slow or fails. */
export function rowDetails(model: Model): ModelDetails {
	return {
		override: emptyLayer(),
		provider: {
			name: text(model.name),
			description: text(model.description),
			family: text(model.family),
			category: text(model.category),
			context_window: model.context_window ?? null,
			max_input: model.max_input ?? null,
			max_output: model.max_output ?? null,
			tools: model.capabilities?.tools ?? null,
			reasoning: model.capabilities?.reasoning ?? null,
			vision: model.capabilities?.vision ?? null,
			status: text(model.status),
			release_date: text(model.release_date)
		},
		modelsdev: emptyLayer(),
		effective_source: {}
	};
}

/** The API resolves fields in this order. Keep a field's actual source beside the displayed
 *  value, including explicit false and zero overrides. */
export function effectiveDetailValue(
	model: Model,
	details: ModelDetails | null,
	key: keyof DetailLayer
): DetailLayer[typeof key] {
	if (details) {
		const source = details.effective_source[key];
		if (source === 'override' || source === 'provider' || source === 'modelsdev') {
			const sourced = details[source][key];
			if (sourced !== null && sourced !== undefined) return sourced;
		}
		const fallback = details.override[key] ?? details.provider[key] ?? details.modelsdev[key];
		if (fallback !== null && fallback !== undefined) return fallback;
	}
	return rowDetails(model).provider[key];
}

function text(value: string | null | undefined): string | null {
	const trimmed = (value ?? '').trim();
	return trimmed === '' ? null : trimmed;
}
