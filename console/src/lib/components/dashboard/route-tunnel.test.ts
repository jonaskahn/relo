import { render } from 'svelte/server';
import { addMessages, init } from 'svelte-i18n';
import { beforeAll, describe, expect, it } from 'vitest';

import en from '$lib/i18n/locales/en.json';
import { providerOf } from '$lib/provider-fixtures';
import { RAIL_LIMIT, routeTunnel, type TunnelContext, type TunnelTraffic } from '$lib/route-tunnel';
import type { AccessKey, UsageLogRow, UsageRollupRow } from '$lib/types';

import RouteTunnel from './route-tunnel.svelte';

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

function row(overrides: Partial<UsageLogRow> = {}): UsageLogRow {
	return {
		ID: 1,
		RequestID: 'r1',
		TimestampMs: 1000,
		Provider: 'openai',
		Model: 'gpt-5',
		RequestedModel: 'gpt-5',
		GroupID: null,
		CredentialLabel: 'work',
		Surface: 'chat',
		Status: 200,
		DurationMs: 1500,
		InputTokens: 1000,
		OutputTokens: 500,
		CacheReadTokens: 200,
		CacheWriteTokens: 100,
		EstimatedCostMicros: 1_200_000,
		RouteProvider: 'openai',
		RouteReason: '',
		ClientKeyID: 'key-1',
		ClientKeyName: 'Codex',
		ClientApp: 'codex',
		Origin: 'external',
		...overrides
	};
}

function key(name: string, client?: string): AccessKey {
	return {
		id: 'id-' + name,
		name,
		kind: 'agent',
		client,
		token_hint: 'sk-relo-…',
		generation: 1,
		status: 'active',
		created_at_ms: 0,
		updated_at_ms: 0
	};
}

function context(overrides: Partial<TunnelContext> = {}): TunnelContext {
	return {
		keys: [key('Codex', 'codex')],
		providers: [providerOf({ id: 'openai', template_id: 'openai', label: 'OpenAI' })],
		console: 'Console',
		limit: RAIL_LIMIT,
		...overrides
	};
}

function traffic(overrides: Partial<TunnelTraffic> = {}): TunnelTraffic {
	return {
		clients: [rollup('Codex', 10)],
		connections: [rollup('openai', 10)],
		newest: row(),
		since: 0,
		...overrides
	};
}

function markup(vertical = false, live = true): string {
	const tunnel = routeTunnel(traffic(), context());
	return render(RouteTunnel, {
		props: { tunnel, width: 900, height: 360, vertical, live }
	}).body;
}

describe('the routing tunnel', () => {
	it('names the agent and the connection it reached', () => {
		const html = markup();

		expect(html).toContain('Codex');
		expect(html).toContain('OpenAI');
	});

	it('draws the newest route in the accent', () => {
		const html = markup();

		expect(html).toContain('route-flow');
		expect(html).toContain('route-moving');
	});

	it('holds the flow still when the tail is not live', () => {
		const html = markup(false, false);

		expect(html).toContain('route-flow');
		expect(html).not.toContain('route-moving');
	});

	it('draws the rails upright on a phone', () => {
		const html = markup(true);

		expect(html).toContain('Codex');
		expect(html).toContain('OpenAI');
	});

	it('names no rail of its own', () => {
		const html = markup();

		expect(html).not.toContain('Agents');
		expect(html).not.toContain('Connections');
	});
});
