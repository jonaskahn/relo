import { render } from 'svelte/server';
import { addMessages, init } from 'svelte-i18n';
import { beforeAll, describe, expect, it } from 'vitest';

import en from '$lib/i18n/locales/en.json';
import { accountOf, providerOf } from '$lib/provider-fixtures';
import type { Account, Provider, QuotaWindow } from '$lib/types';

import AccountsTab from './accounts-tab.svelte';

// An account that has to sign in again offers that action where the quota
// would be, and a meter names how full the window is and when it resets.

beforeAll(() => {
	addMessages('en', en);
	init({ fallbackLocale: 'en', initialLocale: 'en' });
});

const provider = providerOf({
	id: 'claude',
	label: 'Claude',
	auth: 'oauth',
	login_flows: ['claude'],
	login_methods: [{ flow: 'claude', kind: 'browser' }]
});

function markup(
	account: Account,
	windows: QuotaWindow[] = [],
	connection: Provider = provider
): string {
	return render(AccountsTab, {
		props: {
			provider: connection,
			accounts: [account],
			windows,
			onreload: () => {},
			onremoved: () => {},
			onaddkey: () => {}
		}
	}).body;
}

describe('the account card', () => {
	it('offers sign in again in place of the quota when the account needs one', () => {
		const html = markup(accountOf({ kind: 'oauth', status: 'needs_reauth', label: 'work' }));

		expect(html.match(/Sign in again/g)?.length).toBe(2);
		expect(html).not.toContain('role="progressbar"');
		expect(html).not.toContain('No quota data yet');
	});

	it('offers sign in again when the connection’s last refresh failed', () => {
		const html = markup(
			accountOf({ kind: 'oauth', status: 'active', label: 'work' }),
			[],
			providerOf({
				...provider,
				last_refresh_error: 'discover the xAI endpoints: call auth.x.ai: connection refused'
			})
		);

		expect(html).toContain('Active');
		expect(html.match(/Sign in again/g)?.length).toBe(1);
		expect(html).not.toContain('role="progressbar"');
	});

	it('reserves four quota tracks when an account has no windows', () => {
		const html = markup(accountOf({ kind: 'oauth', status: 'active', label: 'work' }));

		expect(html.match(/data-quota-track/g)?.length).toBe(4);
		expect(html).not.toContain('role="progressbar"');
		expect(html).toContain('No quota data yet');
	});

	it('shows a load meter for a healthy account', () => {
		const resetAt = Date.now() + (6 * 24 + 13) * 3_600_000;
		const html = markup(accountOf({ id: 'acc-1', kind: 'oauth', status: 'active' }), [
			{
				credential_id: 'acc-1',
				connection_id: 'claude',
				label: 'work',
				window: '7d',
				window_seconds: 7 * 24 * 3600,
				used_percent: 40,
				reset_at_ms: resetAt,
				source: 'probe',
				observed_at_ms: Date.now(),
				stale: false
			}
		]);

		expect(html).toContain('role="progressbar"');
		expect(html).toContain('40%');
		expect(html).toContain('Weekly');
		expect(html).toContain('6d 13h');
		expect(html).not.toContain('Weekly: Resets in');
		expect(html).toContain('title="Resets in 6d 13h"');
	});

	it('shows money without treating it as a percent window', () => {
		const html = markup(accountOf({ id: 'acc-1', kind: 'api_key', status: 'active' }), [
			{
				credential_id: 'acc-1',
				connection_id: 'deepseek',
				label: 'work',
				window: 'balance',
				window_seconds: 0,
				used_percent: 0,
				amount: 49.5,
				currency: 'USD',
				reset_at_ms: 0,
				source: 'probe',
				observed_at_ms: Date.now(),
				stale: false
			}
		]);

		expect(html).toContain('Balance');
		expect(html).toContain('$49.50');
		expect(html).not.toContain('role="progressbar"');
		expect(html).not.toContain('0%');
	});
});
