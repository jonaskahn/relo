import { render } from 'svelte/server';
import { addMessages, init } from 'svelte-i18n';
import { beforeAll, describe, expect, it } from 'vitest';

import en from '$lib/i18n/locales/en.json';

import RoutingFlow from './routing-flow.svelte';

beforeAll(() => {
	addMessages('en', en);
	init({ fallbackLocale: 'en', initialLocale: 'en' });
});

describe('the routing-flow diagram', () => {
	it('is an accessible image with a title and description', () => {
		const html = render(RoutingFlow, {
			props: {
				groupLabel: 'Fast',
				requestLabel: 'fast',
				members: [
					{
						member: {
							provider_id: 'openai',
							model_id: 'gpt-5',
							weight: 1,
							enabled: true,
							eligible: true
						},
						providerLabel: 'OpenAI',
						modelName: 'GPT-5',
						accounts: [],
						windows: []
					}
				],
				current: 0,
				next: 0,
				hidden: 0,
				chains: []
			}
		}).body;
		expect(html).toContain('role="img"');
		expect(html).toContain('<title id="route-flow-title"');
		expect(html).toContain('<desc id="route-flow-desc"');
	});
});
