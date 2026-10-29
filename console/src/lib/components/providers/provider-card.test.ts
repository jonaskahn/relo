import { render } from 'svelte/server';
import { addMessages, init } from 'svelte-i18n';
import { beforeAll, describe, expect, it } from 'vitest';

import en from '$lib/i18n/locales/en.json';
import { providerOf } from '$lib/provider-fixtures';

import ProviderCard from './provider-card.svelte';

beforeAll(() => {
	addMessages('en', en);
	init({ fallbackLocale: 'en', initialLocale: 'en' });
});

describe('the connection list row', () => {
	it('wears the same billing chip as the card', () => {
		const paidPlan = render(ProviderCard, {
			props: {
				provider: providerOf({
					id: 'alibaba-coding-plan-cn',
					label: 'Alibaba Coding Plan',
					template_id: 'alibaba-coding-plan-cn'
				}),
				onselect: () => {}
			}
		}).body;
		expect(paidPlan).toContain('data-paid-chip');
		expect(paidPlan).toContain('Paid plan');

		const payg = render(ProviderCard, {
			props: { provider: providerOf({ id: 'openai', label: 'OpenAI' }), onselect: () => {} }
		}).body;
		expect(payg).toContain('data-payg-chip');
		expect(payg).toContain('Pay as you go');

		const free = render(ProviderCard, {
			props: {
				provider: providerOf({
					id: 'kilo',
					label: 'Kilo Code Free',
					template_id: 'kilo-free',
					auth: 'none'
				}),
				onselect: () => {}
			}
		}).body;
		expect(free).toContain('data-free-chip');
		expect(free).toContain('Free');
	});

	it('leaves a local endpoint without a chip', () => {
		const html = render(ProviderCard, {
			props: {
				provider: providerOf({
					id: 'ollama',
					label: 'Ollama',
					template_id: 'ollama',
					kind: 'local'
				}),
				onselect: () => {}
			}
		}).body;
		expect(html).not.toContain('data-free-chip');
		expect(html).not.toContain('data-paid-chip');
		expect(html).not.toContain('data-payg-chip');
	});
});
