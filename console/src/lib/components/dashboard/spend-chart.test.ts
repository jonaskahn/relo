import { render } from 'svelte/server';
import { addMessages, init } from 'svelte-i18n';
import { beforeAll, describe, expect, it } from 'vitest';

import en from '$lib/i18n/locales/en.json';
import type { SpendRow } from '$lib/dashboard';
import { providerOf } from '$lib/provider-fixtures';
import type { Provider } from '$lib/types';

import SpendChart from './spend-chart.svelte';

beforeAll(() => {
	addMessages('en', en);
	init({ fallbackLocale: 'en', initialLocale: 'en' });
});

function row(
	id: string,
	label: string,
	costMicros: number,
	unpriced = 0,
	provider: Partial<Provider> = {}
): SpendRow {
	return {
		provider: providerOf({ id, template_id: id, label, kind: 'key', ...provider }),
		requests: 12,
		errors: 0,
		costMicros,
		unpriced,
		tokens: 0
	};
}

function markup(rows: SpendRow[]): string {
	return render(SpendChart, {
		props: {
			rows,
			titleKey: 'ui.pages.dashboard.spendApi',
			descriptionKey: 'ui.pages.dashboard.spendApiDesc',
			emptyKey: 'ui.pages.dashboard.spendEmpty',
			loading: false,
			pending: undefined as never
		}
	}).body;
}

describe('the spend chart', () => {
	it('names the connection and what it cost', () => {
		const html = markup([row('openai', 'OpenAI', 1_200_000)]);

		expect(html).toContain('API spend');
		expect(html).toContain('OpenAI');
		expect(html).toContain('$1.20');
	});

	it('names the costliest connections and marks the rest as more', () => {
		const html = markup([
			row('openai', 'OpenAI', 100),
			row('anthropic', 'Anthropic', 5_000_000),
			row('google', 'Google', 900_000),
			row('mistral', 'Mistral', 700_000)
		]);

		expect(html).toContain('Anthropic');
		expect(html).toContain('Google');
		expect(html).toContain('Mistral');
		expect(html).not.toContain('OpenAI');
		expect(html).toContain('3+');
	});

	it('names how the connection is priced', () => {
		const html = markup([row('openai', 'OpenAI', 1_200_000)]);

		expect(html).toContain('Pay-as-you-go');
	});

	it('names a free lane as free', () => {
		const html = markup([
			row('opencode', 'OpenCode', 0, 0, {
				template_id: 'opencode-free',
				auth: 'none'
			})
		]);

		expect(html).toContain('Free');
	});

	it('names the connections that could not be priced', () => {
		const html = markup([row('openai', 'OpenAI', 1_200_000, 2)]);

		expect(html).toContain('2 unpriced');
	});

	it('says so when the window held no spend', () => {
		const html = markup([]);

		expect(html).toContain('No spend on paid connections');
	});
});
