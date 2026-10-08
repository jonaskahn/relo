import { render } from 'svelte/server';
import { addMessages, init } from 'svelte-i18n';
import { beforeAll, describe, expect, it } from 'vitest';

import en from '$lib/i18n/locales/en.json';
import { accountOf, providerOf, quotaWindowOf } from '$lib/provider-fixtures';
import type { Account, Provider, QuotaWindow } from '$lib/types';

import ProviderQuotaFooter from './provider-quota-footer.svelte';

// The overview card footer numbers every account, warns on one that needs a
// new sign-in, and puts that action where the quota would be.

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
	accounts: Account[],
	windows: QuotaWindow[] = [],
	connection: Provider = provider,
	layout: 'vertical' | 'horizontal' = 'vertical'
): string {
	return render(ProviderQuotaFooter, {
		props: {
			provider: connection,
			accounts,
			windows,
			layout,
			needsCredentials: true,
			onreload: () => {}
		}
	}).body;
}

describe('the connection card footer', () => {
	it('reads an unlimited quota when the provider needs no account', () => {
		const html = render(ProviderQuotaFooter, {
			props: {
				provider: providerOf({ id: 'opencode-free', auth: 'none', label: 'Opencode Free' }),
				accounts: [],
				needsCredentials: false,
				onreload: () => {}
			}
		}).body;

		expect(html).toContain('∞');
		expect(html).toContain('Unlimited');
		expect(html).toContain('Account 1: Opencode Free');
		expect(html).not.toContain('This provider needs no credentials.');
	});

	it('keeps the unlimited reading and numbers each free account', () => {
		const html = render(ProviderQuotaFooter, {
			props: {
				provider: providerOf({ id: 'opencode-free', auth: 'none', label: 'Opencode Free' }),
				accounts: [
					accountOf({ id: 'acc-1', kind: 'api_key', label: 'one' }),
					accountOf({ id: 'acc-2', kind: 'api_key', label: 'two' })
				],
				needsCredentials: false,
				onreload: () => {}
			}
		}).body;

		expect(html).toContain('Account 1: one');
		expect(html).toContain('Account 2: two');
		expect(html).toContain('∞');
		expect(html).not.toContain('No quota data yet');
	});

	it('numbers a single account', () => {
		const html = markup([accountOf({ kind: 'oauth', status: 'active', label: 'work' })]);

		expect(html).toContain('Account 1: work');
	});

	it('keeps eight numbers in view on More and ten on Less', () => {
		const accounts = Array.from({ length: 12 }, (_, index) =>
			accountOf({ id: 'acc-' + (index + 1), label: 'a' + (index + 1) })
		);

		expect(markup(accounts)).toContain('min(100%, 16.875rem)');
		expect(markup(accounts, [], provider, 'horizontal')).toContain('min(100%, 21.125rem)');
	});

	it('reserves four quota tracks when an account has no windows', () => {
		const html = markup([accountOf({ kind: 'oauth', status: 'active', label: 'work' })]);

		expect(html.match(/data-quota-track/g)?.length).toBe(4);
		expect(html).not.toContain('role="progressbar"');
		expect(html).toContain('No quota data yet');
	});

	it('warns on a number that needs a new sign-in and offers that action instead of quota', () => {
		const html = markup(
			[accountOf({ id: 'acc-2', kind: 'oauth', status: 'needs_reauth', label: 'stale' })],
			[
				quotaWindowOf({
					credential_id: 'acc-2',
					connection_id: 'claude',
					label: 'stale',
					used_percent: 20
				})
			]
		);

		expect(html).toContain('Account 1: stale — needs sign-in');
		expect(html).toContain('Sign in again');
		expect(html).not.toContain('role="progressbar"');
	});

	it('keeps quota on a healthy account while marking the one that needs sign-in', () => {
		const html = markup(
			[
				accountOf({ id: 'acc-1', kind: 'oauth', status: 'active', label: 'ok' }),
				accountOf({ id: 'acc-2', kind: 'oauth', status: 'needs_reauth', label: 'stale' })
			],
			[
				quotaWindowOf({
					credential_id: 'acc-1',
					connection_id: 'claude',
					label: 'ok',
					used_percent: 20
				})
			]
		);

		expect(html).toContain('Account 1: ok');
		expect(html).toContain('Account 2: stale — needs sign-in');
		expect(html).toContain('role="progressbar"');
		expect(html).toContain('20%');
		expect(html).not.toContain('Sign in again');
	});

	it('shows a load meter for a healthy selected account', () => {
		const resetAt = Date.now() + (6 * 24 + 13) * 3_600_000;
		const html = markup(
			[accountOf({ id: 'acc-1', kind: 'oauth', status: 'active', label: 'work' })],
			[
				quotaWindowOf({
					credential_id: 'acc-1',
					connection_id: 'claude',
					label: 'work',
					window: '7d',
					used_percent: 40,
					reset_at_ms: resetAt
				})
			]
		);

		expect(html).toContain('role="progressbar"');
		expect(html).toContain('40%');
		expect(html).toContain('Weekly');
		expect(html).toContain('6d 13h');
		expect(html).not.toContain('Sign in again');
	});
});
