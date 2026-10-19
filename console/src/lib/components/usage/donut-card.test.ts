import { render } from 'svelte/server';
import { addMessages, init } from 'svelte-i18n';
import { beforeAll, describe, expect, it } from 'vitest';

import en from '$lib/i18n/locales/en.json';
import type { DonutSegment } from '$lib/usage-charts';

import DonutCard from './donut-card.svelte';

beforeAll(() => {
	addMessages('en', en);
	init({ fallbackLocale: 'en', initialLocale: 'en' });
});

const segments: DonutSegment[] = [
	{ key: 'openai', value: 2_000_000, share: 60, start: 0, sweep: 60 },
	{ key: 'claude', value: 1_000_000, share: 30, start: 60, sweep: 30 },
	{ key: 'localhost', value: 333_333, share: 10, start: 90, sweep: 10 }
];

function markup(): string {
	return render(DonutCard, {
		props: {
			titleKey: 'ui.pages.usagePage.tokenByProviderTitle',
			segments,
			totalText: '3.33M',
			captionOf: (key: string) => (key === 'openai' ? 'OpenAI' : key),
			logoOf: (key: string) => key,
			valueOf: (value: number) => value / 1_000_000 + 'M',
			centerKey: 'ui.pages.usagePage.summaryTotalTokens'
		}
	}).body;
}

describe('the provider donut', () => {
	it('states the ring total at rest', () => {
		expect(markup()).toContain('>3.33M<');
	});

	it('keeps the provider name in the legend instead of stretching the caption', () => {
		const html = markup();
		expect(html).toContain('>OpenAI</span>');
		expect(html).not.toContain('textLength');
	});
});
