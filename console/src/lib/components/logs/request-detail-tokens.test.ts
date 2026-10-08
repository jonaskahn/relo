import { render } from 'svelte/server';
import { addMessages, init } from 'svelte-i18n';
import { beforeAll, describe, expect, it } from 'vitest';

import en from '$lib/i18n/locales/en.json';
import { modelOf } from '$lib/provider-fixtures';
import type { Model, UsageLogRow } from '$lib/types';

import RequestDetailTokens from './request-detail-tokens.svelte';

// The token table is what one request cost and what it was billed at. The rates
// come from the model as the console reads it now, so what is under test is
// that a row states the rate and the cost it computes, and that a request with
// no model behind it says so rather than showing a number it cannot stand by.

beforeAll(() => {
	addMessages('en', en);
	init({ fallbackLocale: 'en', initialLocale: 'en' });
});

function row(overrides: Partial<UsageLogRow> = {}): UsageLogRow {
	return {
		ID: 1,
		RequestID: 'r1',
		TimestampMs: 0,
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
		ClientKeyID: '',
		ClientKeyName: '',
		Origin: 'external',
		...overrides
	};
}

function markup(r: UsageLogRow, model: Model | null, loading = false): string {
	return render(RequestDetailTokens, { props: { row: r, model, loading } }).body;
}

describe('the token table', () => {
	it('states the four categories a request is billed in', () => {
		const html = markup(row(), modelOf());
		for (const label of ['Input', 'Output', 'Cache read', 'Cache write', 'Total']) {
			expect(html).toContain('>' + label + '<');
		}
	});

	it('bills the plain input, leaving the cached tokens to their own rows', () => {
		// 1000 input tokens less 200 read and 100 written is 700 at the input
		// rate, which is what the row states rather than the 1000 the log holds.
		const html = markup(row(), modelOf());
		expect(html).toContain('>700<');
		expect(html).toContain('>500<');
		expect(html).toContain('>200<');
		expect(html).toContain('>100<');
	});

	it('totals the tokens the request carried and the cost it recorded', () => {
		const html = markup(row(), modelOf());
		expect(html).toContain('>1,500<');
	});

	it('says the rates are unavailable rather than implying a price', () => {
		const html = markup(row(), null);
		expect(html).toContain('Rates unavailable');
		// Every rate cell reads as a dash, so the table never states a price the
		// console does not know.
		expect(html.match(/>—<\/td>/g)).toHaveLength(4);
	});

	it('states the rate and the cost a known model bills each category at', () => {
		const html = markup(row(), modelOf());
		// The fixture prices input at 1.25 per million and output at 10, which is
		// what the row states beside the tokens it uses.
		expect(html).toContain('$1.25');
		expect(html).toContain('$10.00');
	});
});
