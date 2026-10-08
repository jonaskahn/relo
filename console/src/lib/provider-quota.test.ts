import { describe, expect, it } from 'vitest';

import {
	QUOTA_RINGS,
	groupAccountsByConnection,
	groupQuotaByConnection,
	isMoneyReading,
	quotaWindowLabelKey,
	ringDash,
	ringsOf,
	selectedAccount,
	shownPercent,
	windowsByCredential
} from './provider-quota';
import { accountOf, quotaWindowOf } from './provider-fixtures';

describe('windowsByCredential', () => {
	it('groups the windows each account reported', () => {
		const grouped = windowsByCredential([
			quotaWindowOf({ credential_id: 'a1', window: '5h' }),
			quotaWindowOf({ credential_id: 'a1', window: '7d' }),
			quotaWindowOf({ credential_id: 'a2', window: '5h' })
		]);

		expect(grouped.get('a1')?.map((window) => window.window)).toEqual(['5h', '7d']);
		expect(grouped.get('a2')).toHaveLength(1);
		expect(grouped.get('a3')).toBeUndefined();
	});
});

describe('groupAccountsByConnection', () => {
	it('keeps the accounts of one connection together, in daemon order', () => {
		const grouped = groupAccountsByConnection([
			accountOf({ id: 'a1', provider_id: 'openai', label: 'work' }),
			accountOf({ id: 'b1', provider_id: 'anthropic', label: 'personal' }),
			accountOf({ id: 'a2', provider_id: 'openai', label: 'backup' })
		]);

		expect(grouped.get('openai')?.map((account) => account.id)).toEqual(['a1', 'a2']);
		expect(grouped.get('ollama')).toBeUndefined();
	});
});

describe('shownPercent', () => {
	it('reads the share used, which is what the daemon reports', () => {
		expect(shownPercent(30, 'used')).toBe(30);
	});

	it('reads what is left when the console asks for it', () => {
		expect(shownPercent(30, 'remaining')).toBe(70);
		expect(shownPercent(0, 'remaining')).toBe(100);
	});

	it('clamps a reading that went past its own limit', () => {
		expect(shownPercent(120, 'used')).toBe(100);
		expect(shownPercent(120, 'remaining')).toBe(0);
		expect(shownPercent(-5, 'remaining')).toBe(100);
	});
});

describe('groupQuotaByConnection', () => {
	it('indexes the windows a connection published', () => {
		const grouped = groupQuotaByConnection([
			quotaWindowOf({ connection_id: 'openai', credential_id: 'a1' }),
			quotaWindowOf({ connection_id: 'anthropic', credential_id: 'b1' }),
			quotaWindowOf({ connection_id: 'openai', credential_id: 'a2' })
		]);

		expect(grouped.get('openai')).toHaveLength(2);
		expect(grouped.get('anthropic')).toHaveLength(1);
	});
});

describe('quotaWindowLabelKey', () => {
	it('names the two windows a provider usually publishes', () => {
		expect(quotaWindowLabelKey('5h')).toBe('ui.pages.providersPage.accounts.quotaWindowHours');
		expect(quotaWindowLabelKey('7d')).toBe('ui.pages.providersPage.accounts.quotaWindowWeekly');
	});

	it('leaves an unknown window to the tag the provider used', () => {
		expect(quotaWindowLabelKey('30d')).toBe('ui.pages.providersPage.accounts.quotaWindowMonthly');
		expect(quotaWindowLabelKey('daily')).toBe('ui.pages.providersPage.accounts.quotaWindowDaily');
		expect(quotaWindowLabelKey('credits')).toBe(
			'ui.pages.providersPage.accounts.quotaWindowCredits'
		);
		expect(quotaWindowLabelKey('key')).toBe('ui.pages.providersPage.accounts.quotaWindowKey');
		expect(quotaWindowLabelKey('balance')).toBe(
			'ui.pages.providersPage.accounts.quotaWindowBalance'
		);
		expect(quotaWindowLabelKey('balance-usd')).toBe(
			'ui.pages.providersPage.accounts.quotaWindowBalance'
		);
		expect(quotaWindowLabelKey('spent')).toBe('ui.pages.providersPage.accounts.quotaWindowSpent');
		expect(quotaWindowLabelKey('Sonnet')).toBe('');
	});
});

describe('isMoneyReading', () => {
	it('requires an amount and currency', () => {
		expect(isMoneyReading(quotaWindowOf({ amount: 0, currency: 'USD' }))).toBe(true);
		expect(isMoneyReading(quotaWindowOf({ amount: 0 }))).toBe(false);
		expect(isMoneyReading(quotaWindowOf({}))).toBe(false);
	});
});

describe('ringsOf', () => {
	it('draws four windows and counts the rest', () => {
		const windows = ['1h', '5h', '1d', '7d', '30d'].map((window) => quotaWindowOf({ window }));

		const rings = ringsOf(windows);

		expect(QUOTA_RINGS).toBe(4);
		expect(rings.shown).toHaveLength(4);
		expect(rings.extra).toBe(1);
	});

	it('counts nothing when every window fits', () => {
		expect(ringsOf([quotaWindowOf({}), quotaWindowOf({ window: '7d' })]).extra).toBe(0);
	});
});

describe('selectedAccount', () => {
	const accounts = [accountOf({ id: 'a1' }), accountOf({ id: 'a2' })];

	it('reads the account the index names', () => {
		expect(selectedAccount(accounts, 1)?.id).toBe('a2');
	});

	it('falls back to the first account when the list moved under it', () => {
		expect(selectedAccount(accounts, 4)?.id).toBe('a1');
		expect(selectedAccount(accounts, -1)?.id).toBe('a1');
	});

	it('selects nothing for a connection with no accounts', () => {
		expect(selectedAccount([], 0)).toBeNull();
	});
});

describe('ringDash', () => {
	const circumference = 2 * Math.PI * 28;

	it('leaves nothing undrawn for a full window', () => {
		expect(ringDash(100, circumference)).toBe(0);
		expect(ringDash(0, circumference)).toBe(circumference);
	});

	it('clamps a share outside 0-100 rather than drawing past the circle', () => {
		expect(ringDash(140, circumference)).toBe(0);
		expect(ringDash(-20, circumference)).toBe(circumference);
	});
});
