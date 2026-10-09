import { render } from 'svelte/server';
import { addMessages, init } from 'svelte-i18n';
import { beforeAll, describe, expect, it } from 'vitest';

import en from '$lib/i18n/locales/en.json';
import { accountOf, providerOf, quotaWindowOf } from '$lib/provider-fixtures';

import ProviderOverviewCard from './provider-overview-card.svelte';

beforeAll(() => {
	addMessages('en', en);
	init({ fallbackLocale: 'en', initialLocale: 'en' });
});

describe('the connection overview card', () => {
	it('names the selected account in the header', () => {
		const html = render(ProviderOverviewCard, {
			props: {
				provider: providerOf({ id: 'claude', label: 'Claude', auth: 'oauth' }),
				accounts: [
					accountOf({ id: 'acc-1', kind: 'oauth', label: 'work' }),
					accountOf({ id: 'acc-2', kind: 'oauth', label: 'home' })
				],
				onopen: () => {}
			}
		}).body;

		expect(html).toContain('data-selected-account="work"');
		expect(html).toContain('Claude');
		expect(html).toContain('Account 1: work');
		expect(html).toContain('Account 2: home');
	});

	it('keeps the account line when a provider has no account', () => {
		const html = render(ProviderOverviewCard, {
			props: {
				provider: providerOf({
					id: 'opencode-free',
					label: 'Opencode Free',
					auth: 'none',
					counts: { ...providerOf().counts, accounts: 0, active_accounts: 0 }
				}),
				accounts: [],
				onopen: () => {}
			}
		}).body;

		expect(html).toContain('data-account-line');
		expect(html).not.toContain('data-selected-account=');
		expect(html).toContain('data-status-pill="ready"');
		expect(html).toContain('Account 1: Opencode Free');
		expect(html).toContain('∞');
		expect(html).toContain('Unlimited');
		expect(html).not.toContain('This provider needs no credentials.');
	});

	it('numbers accounts on a connection whose quota is unlimited', () => {
		const html = render(ProviderOverviewCard, {
			props: {
				provider: providerOf({
					id: 'opencode-free',
					label: 'Opencode Free',
					auth: 'none',
					counts: { ...providerOf().counts, accounts: 2, active_accounts: 2 }
				}),
				accounts: [
					accountOf({ id: 'acc-1', kind: 'api_key', label: 'one' }),
					accountOf({ id: 'acc-2', kind: 'api_key', label: 'two' })
				],
				onopen: () => {}
			}
		}).body;

		expect(html).toContain('3/3 models on');
		expect(html).toContain('Account 1: one');
		expect(html).toContain('Account 2: two');
		expect(html).toContain('∞');
	});

	// A keyless pool bills nothing because it accepts no account, so the card
	// says so rather than leaving the operator to infer it from the price.
	it('chips a keyless pool as free and leaves a metered one bare', () => {
		const free = render(ProviderOverviewCard, {
			props: {
				provider: providerOf({
					id: 'kilo',
					label: 'Kilo Free',
					template_id: 'kilo-free',
					auth: 'none'
				}),
				onopen: () => {}
			}
		}).body;
		expect(free).toContain('data-free-chip');

		const metered = render(ProviderOverviewCard, {
			props: { provider: providerOf({ id: 'openai', label: 'OpenAI' }), onopen: () => {} }
		}).body;
		expect(metered).not.toContain('data-free-chip');
	});

	it('raises the status pill with the account quota', () => {
		const html = render(ProviderOverviewCard, {
			props: {
				provider: providerOf({ id: 'claude', label: 'Claude', auth: 'oauth' }),
				accounts: [accountOf({ id: 'acc-1', kind: 'oauth', label: 'work' })],
				windows: [
					quotaWindowOf({ connection_id: 'claude', credential_id: 'acc-1', used_percent: 100 })
				],
				onopen: () => {}
			}
		}).body;

		expect(html).toContain('data-status-pill="limitReached"');
		expect(html).toContain('Limit reached');
		expect(html).toContain('card-limit');
	});
});
