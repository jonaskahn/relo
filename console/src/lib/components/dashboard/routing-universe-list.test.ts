import { render } from 'svelte/server';
import { addMessages, init } from 'svelte-i18n';
import { beforeAll, describe, expect, it } from 'vitest';

import en from '$lib/i18n/locales/en.json';
import { RAIL_LIMIT, routeTunnel, type TunnelContext, type TunnelTraffic } from '$lib/route-tunnel';
import type { UsageRollupRow } from '$lib/types';

import RoutingUniverseList from './routing-universe-list.svelte';

beforeAll(() => {
	addMessages('en', en);
	init({ fallbackLocale: 'en', initialLocale: 'en' });
});

function rollup(key: string, requests: number): UsageRollupRow {
	return {
		Key: key,
		Requests: requests,
		Errors: 0,
		InputTokens: 100,
		OutputTokens: 20,
		CacheReadTokens: 5,
		CacheWriteTokens: 5,
		CostMicros: 1000,
		UnpricedRequests: 0,
		DurationMs: 0,
		Attempts: 0,
		RetriedRequests: 0,
		DurationMaxMs: 0
	};
}

function context(): TunnelContext {
	return { keys: [], providers: [], console: 'Console', limit: RAIL_LIMIT };
}

function traffic(overrides: Partial<TunnelTraffic> = {}): TunnelTraffic {
	return {
		clients: [rollup('Codex', 10)],
		connections: [rollup('openai', 10), rollup('anthropic', 4)],
		since: 0,
		...overrides
	};
}

function markup(trafficRows: TunnelTraffic): string {
	const tunnel = routeTunnel(trafficRows, context());
	return render(RoutingUniverseList, { props: { tunnel } }).body;
}

describe('the routing universe list', () => {
	it('names both rails and what they carried', () => {
		const html = markup(traffic());

		expect(html).toContain('Clients');
		expect(html).toContain('Providers');
		expect(html).toContain('Codex');
		expect(html).toContain('openai');
		expect(html).toContain('10');
		expect(html).toContain('4');
	});

	it('draws each node against the busiest node on its rail', () => {
		const html = markup(traffic());

		expect(html).toContain('width: 100.0%');
		expect(html).toContain('width: 40.0%');
	});
});
