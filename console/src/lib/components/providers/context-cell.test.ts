import { render } from 'svelte/server';
import { addMessages, init } from 'svelte-i18n';
import { beforeAll, describe, expect, it } from 'vitest';

import en from '$lib/i18n/locales/en.json';
import { modelOf } from '$lib/provider-fixtures';
import type { Model } from '$lib/types';

import ContextPicker from './context-picker.svelte';

// The Context cell is a fixed-size readout with a one-character notice, so a
// long custom size cannot widen the column and a value above the model's own
// maximum input is marked rather than hidden.

beforeAll(() => {
	// The app's own initI18n touches the document, which a server render has
	// none of; the catalog is what the rendered titles read.
	addMessages('en', en);
	init({ fallbackLocale: 'en', initialLocale: 'en' });
});

function markup(model: Model, field: 'context' | 'output' = 'context'): string {
	return render(ContextPicker, { props: { model, field, onsave: async () => {} } }).body;
}

describe('the context cell', () => {
	it('holds the value in a fixed box with the notice slot reserved', () => {
		const html = markup(
			modelOf({
				context_window: 800_000,
				context_layers: { override: null, provider: 800_000, modelsdev: null }
			})
		);

		expect(html).toContain('w-[7ch]');
		expect(html).toContain('w-[1ch]');
		expect(html).toContain('800K');
		expect(html).toContain('title="800K"');
	});

	it('marks a value the operator set', () => {
		const html = markup(
			modelOf({
				context_window: 200_000,
				max_input: 262_144,
				context_layers: { override: 200_000, provider: null, modelsdev: null }
			})
		);

		expect(html).toContain('>*</span>');
		expect(html).toContain('text-accent-strong');
	});

	it('offers the same presets for max output', () => {
		const html = markup(
			modelOf({
				max_output: 128_000,
				max_output_layers: { override: 128_000, provider: 64_000, modelsdev: null },
				max_output_source: 'override'
			}),
			'output'
		);

		expect(html).toContain('Set a max output override (currently 128K)');
		expect(html).toContain('128K');
		expect(html).toContain('>*</span>');
	});

	it('warns when the override is above the maximum input the model states', () => {
		const html = markup(
			modelOf({
				context_window: 1_000_000,
				max_input: 262_144,
				context_layers: { override: 1_000_000, provider: null, modelsdev: null }
			})
		);

		expect(html).toContain('>!</span>');
		expect(html).toContain('Above the model’s maximum input of 262k');
	});
});
