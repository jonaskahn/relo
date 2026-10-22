import { render } from 'svelte/server';
import { addMessages, init } from 'svelte-i18n';
import { beforeAll, describe, expect, it } from 'vitest';

import en from '$lib/i18n/locales/en.json';
import type { AccountAttention } from '$lib/dashboard';
import type { Account, QuotaWindow } from '$lib/types';

import AttentionBand from './attention-band.svelte';

beforeAll(() => {
	addMessages('en', en);
	init({ fallbackLocale: 'en', initialLocale: 'en' });
});

function account(label: string): Account {
	return {
		id: 'a1',
		provider_id: 'openai',
		kind: 'api_key',
		label,
		status: 'active',
		priority: 0
	};
}

function quotaWindow(usedPercent: number): QuotaWindow {
	return {
		credential_id: 'a1',
		connection_id: 'openai',
		label: 'work',
		window: '5h',
		window_seconds: 18000,
		used_percent: usedPercent,
		reset_at_ms: 0,
		source: 'probe',
		observed_at_ms: 0,
		stale: false
	};
}

function item(overrides: Partial<AccountAttention> = {}): AccountAttention {
	return {
		account: account('hi@claude.ai'),
		providerId: 'openai',
		reason: 'reached',
		window: quotaWindow(100),
		tone: 'danger',
		...overrides
	};
}

function markup(items: AccountAttention[]): string {
	return render(AttentionBand, { props: { items, onopen: () => {} } }).body;
}

describe('the attention band', () => {
	it('says nothing when no account needs attention', () => {
		const html = markup([]);

		expect(html).not.toContain('needs attention');
		expect(html).not.toContain('hi@claude.ai');
	});

	it('names the account and why it needs an operator', () => {
		const html = markup([item()]);

		expect(html).toContain('hi@claude.ai');
		expect(html).toContain('has reached its');
		expect(html).toContain('1 needs attention');
	});

	it('names a sign-in that expired', () => {
		const html = markup([item({ reason: 'needs_signin', window: undefined, tone: 'danger' })]);

		expect(html).toContain('needs a new sign-in');
	});

	it('keeps the accounts on one scrolling row', () => {
		const html = markup([item()]);

		expect(html).toContain('attention-viewport');
		expect(html).toContain('attention-track');
	});

	it('repeats the row once so the scroll loops without a seam', () => {
		const html = markup([item()]);

		expect(html.match(/hi@claude\.ai/g)?.length).toBe(2);
		expect(html).toContain('tabindex="-1"');
	});
});
