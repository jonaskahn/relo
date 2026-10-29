import { render } from 'svelte/server';
import { addMessages, init } from 'svelte-i18n';
import { beforeAll, describe, expect, it } from 'vitest';

import en from '$lib/i18n/locales/en.json';
import { templateOf } from '$lib/provider-fixtures';

import StepProvider from './step-provider.svelte';

beforeAll(() => {
	addMessages('en', en);
	init({ fallbackLocale: 'en', initialLocale: 'en' });
});

const templates = [
	templateOf({ id: 'openai', label: 'OpenAI' }),
	templateOf({ id: 'opencode-free', label: 'OpenCode Zen Free' }),
	templateOf({ id: 'kilo-free', label: 'Kilo Code Free' })
];

function renderPicker(freeEnabled: boolean): string {
	return render(StepProvider, {
		props: {
			templates,
			freeEnabled,
			freeBusy: false,
			onfreetoggle: () => {},
			modelsdev: null,
			added: new Set<string>(),
			onquery: () => {},
			onpick: () => {},
			oncustom: () => {},
			onretry: () => {}
		}
	}).body;
}

describe('the free connection picker', () => {
	it('keeps the free heading and toggle while hiding every free row', () => {
		const html = renderPicker(false);
		expect(html).toContain('Free connections');
		expect(html).toContain('data-free-toggle');
		expect(html).not.toContain('OpenCode Zen Free');
		expect(html).not.toContain('Kilo Code Free');
	});

	it('shows both free rows first once enabled', () => {
		const html = renderPicker(true);
		expect(html.indexOf('OpenCode Zen Free')).toBeLessThan(html.indexOf('OpenAI'));
		expect(html).toContain('Kilo Code Free');
	});
});
