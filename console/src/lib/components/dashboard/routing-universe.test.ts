import { render } from 'svelte/server';
import { addMessages, init } from 'svelte-i18n';
import { beforeAll, describe, expect, it } from 'vitest';

import en from '$lib/i18n/locales/en.json';
import { providerOf } from '$lib/provider-fixtures';
import type { TunnelTraffic } from '$lib/route-tunnel';

import RoutingUniverse from './routing-universe.svelte';

beforeAll(() => {
	addMessages('en', en);
	init({ fallbackLocale: 'en', initialLocale: 'en' });
});

function traffic(): TunnelTraffic {
	return { clients: [], connections: [], since: 0 };
}

function markup(providers: ReturnType<typeof providerOf>[]): string {
	return render(RoutingUniverse, {
		props: {
			traffic: traffic(),
			keys: [],
			providers,
			live: false,
			loading: false,
			onadd: () => {}
		}
	}).body;
}

describe('the routing universe', () => {
	it('names itself with an icon', () => {
		const html = markup([]);

		expect(html).toContain('Routing Universe');
	});

	it('offers the action that fills an empty card', () => {
		const html = markup([]);

		expect(html).toContain('No providers configured');
		expect(html).toContain('Add connection');
	});

	it('says so when the window held no traffic', () => {
		const html = markup([providerOf({ id: 'openai', template_id: 'openai', label: 'OpenAI' })]);

		expect(html).toContain('No requests in this period');
	});
});
