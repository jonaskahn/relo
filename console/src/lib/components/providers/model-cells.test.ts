import { render } from 'svelte/server';
import { addMessages, init } from 'svelte-i18n';
import { beforeAll, describe, expect, it } from 'vitest';

import en from '$lib/i18n/locales/en.json';
import { modelOf } from '$lib/provider-fixtures';
import type { Model } from '$lib/types';

import CapabilityPicker from './capability-picker.svelte';

// Every cell of a model row reads the same way: the label states what the
// column holds, the value is what the agent will see, and the quiet trigger
// under it is the only thing that changes a value.

beforeAll(() => {
	// The app's own initI18n touches the document, which a server render has
	// none of; the catalog is what the rendered labels read.
	addMessages('en', en);
	init({ fallbackLocale: 'en', initialLocale: 'en' });
});

function capability(
	model: Model,
	field: 'tools' | 'reasoning' | 'vision',
	labelKey: string
): string {
	return render(CapabilityPicker, {
		props: { model, field, labelKey, onsave: async () => {} }
	}).body;
}

const column: Record<string, string> = {
	tools: 'ui.pages.providersPage.modelsTable.columnTools',
	reasoning: 'ui.pages.providersPage.modelsTable.columnReasoning',
	vision: 'ui.pages.providersPage.modelsTable.columnVision'
};

describe('a capability cell', () => {
	it('labels itself and states the value in words for a reader', () => {
		const html = capability(
			modelOf({ capabilities: { tools: true, reasoning: null, vision: null } }),
			'tools',
			column.tools
		);

		expect(html).toContain('Tools');
		expect(html).toContain('aria-label="Tools: On"');
	});

	it('states a capability nothing knows as default', () => {
		const html = capability(
			modelOf({ capabilities: { tools: null, reasoning: false, vision: null } }),
			'reasoning',
			column.reasoning
		);

		expect(html).toContain('aria-label="Reasoning: Off"');
		expect(html).toContain('Set Reasoning');
	});

	it('marks the value an operator pinned', () => {
		const forced = capability(
			modelOf({
				capabilities: { tools: true, reasoning: null, vision: null },
				capability_override: { tools: true, reasoning: null, vision: null }
			}),
			'tools',
			column.tools
		);
		const left = capability(modelOf(), 'tools', column.tools);

		expect(forced).toContain('>*</span>');
		expect(left).not.toContain('>*</span>');
	});
});
