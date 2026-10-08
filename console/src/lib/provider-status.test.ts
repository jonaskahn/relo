import { describe, expect, test } from 'vitest';

import { accountNeedsSignIn, bannerAction, providerStatus } from './provider-status';
import { accountOf, providerOf } from './provider-fixtures';

describe('provider status', () => {
	test('a paused provider reads paused before anything else', () => {
		const provider = providerOf({
			enabled: false,
			needs_setup: ['region'],
			last_refresh_error: 'boom',
			counts: {
				models: 0,
				enabled_models: 0,
				available_models: 0,
				unpriced_models: 0,
				accounts: 0,
				active_accounts: 0,
				paused_accounts: 0,
				reauth_accounts: 0
			}
		});
		expect(providerStatus(provider).name).toBe('paused');
	});

	test('a missing variable reads needs setup', () => {
		expect(providerStatus(providerOf({ needs_setup: ['resource'] })).name).toBe('needsSetup');
	});

	test('a provider that needs a credential and has none reads no account', () => {
		expect(
			providerStatus(
				providerOf({
					counts: { ...providerOf().counts, accounts: 0, active_accounts: 0, paused_accounts: 0 }
				})
			).name
		).toBe('noAccount');
	});

	test('a keyless provider with no accounts still reads ready when it has models', () => {
		expect(
			providerStatus(providerOf({ auth: 'none', counts: { ...providerOf().counts, accounts: 0 } }))
				.name
		).toBe('ready');
	});

	test('an account that needs a new sign-in reads sign in again', () => {
		expect(
			providerStatus(
				providerOf({
					counts: { ...providerOf().counts, active_accounts: 0, reauth_accounts: 1 }
				})
			).name
		).toBe('signInAgain');
	});

	test('every account paused reads accounts paused', () => {
		expect(
			providerStatus(
				providerOf({
					counts: { ...providerOf().counts, active_accounts: 0, paused_accounts: 1 }
				})
			).name
		).toBe('accountsPaused');
	});

	test('a failed refresh reads model list failed', () => {
		expect(providerStatus(providerOf({ last_refresh_error: '502' })).name).toBe('modelListFailed');
	});

	test('no enabled model reads no models on', () => {
		expect(
			providerStatus(providerOf({ counts: { ...providerOf().counts, enabled_models: 0 } })).name
		).toBe('noModelsOn');
	});

	test('a provider with models reads ready', () => {
		expect(providerStatus(providerOf()).name).toBe('ready');
	});

	test('an oauth account that needs reauth has to sign in again', () => {
		expect(accountNeedsSignIn(accountOf({ kind: 'oauth', status: 'needs_reauth' }))).toBe(true);
		expect(accountNeedsSignIn(accountOf({ kind: 'oauth', status: 'active' }), '502')).toBe(true);
		expect(accountNeedsSignIn(accountOf({ kind: 'oauth', status: 'active' }))).toBe(false);
		expect(accountNeedsSignIn(accountOf({ kind: 'api_key', status: 'needs_reauth' }))).toBe(false);
	});

	test('every status that is not ready offers exactly one action', () => {
		expect(bannerAction('ready')).toBe('');
		for (const name of [
			'paused',
			'needsSetup',
			'noAccount',
			'signInAgain',
			'accountsPaused',
			'modelListFailed',
			'noModelsOn'
		] as const) {
			expect(bannerAction(name)).not.toBe('');
		}
	});
});
