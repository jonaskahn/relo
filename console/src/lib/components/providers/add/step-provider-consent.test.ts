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

describe('the free consent gate in the picker', () => {
	it('keeps the committed switch off while a consent request is pending', () => {
		const html = render(StepProvider, {
			props: {
				templates: [
					templateOf({ id: 'opencode-free', label: 'OpenCode Zen Free' }),
					templateOf({ id: 'openai', label: 'OpenAI' })
				],
				freeEnabled: false,
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
		expect(html).toContain('data-free-toggle');
		expect(html).toContain('aria-checked="false"');
		expect(html).not.toContain('OpenCode Zen Free');
	});

	it('keeps the tracing accent glow in both consent states', () => {
		const base = {
			templates: [templateOf({ id: 'opencode-free', label: 'OpenCode Zen Free' })],
			freeBusy: false,
			onfreetoggle: () => {},
			modelsdev: null,
			added: new Set<string>(),
			onquery: () => {},
			onpick: () => {},
			oncustom: () => {},
			onretry: () => {}
		};
		const off = render(StepProvider, { props: { ...base, freeEnabled: false } }).body;
		const on = render(StepProvider, { props: { ...base, freeEnabled: true } }).body;
		expect(off).toContain('free-heading-glow');
		expect(on).toContain('free-heading-glow');
		expect(off).toContain('free-glow-line');
		expect(on).toContain('free-glow-line');
		expect(on).toContain('data-free-toggle');
	});
});
