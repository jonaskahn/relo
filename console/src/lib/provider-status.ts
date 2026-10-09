import type { Account, Provider } from '$lib/types';

// One word for the state a provider is in, which every card, pane header and
// banner reads. The order below is a priority: the first condition a provider
// meets names it, so a paused provider never also reads as unconfigured.

/** The state a connection reads as, decided by the first condition it meets: the first match in
 *  this union wins. */
export type ProviderStatusName =
	| 'ready'
	| 'paused'
	| 'needsSetup'
	| 'noAccount'
	| 'signInAgain'
	| 'accountsPaused'
	| 'modelListFailed'
	| 'noModelsOn';

/** The shape a status pill wears. */
export type StatusTone = 'pass' | 'warn' | 'muted';

/** Where one connection stands, and what its banner can offer. */
export interface ProviderStatus {
	name: ProviderStatusName;
	tone: StatusTone;
	labelKey: string;
	bannerKey: string;
}

const STATUS: Record<ProviderStatusName, ProviderStatus> = {
	ready: {
		name: 'ready',
		tone: 'pass',
		labelKey: 'ui.pages.providersPage.status.ready',
		bannerKey: ''
	},
	paused: {
		name: 'paused',
		tone: 'muted',
		labelKey: 'ui.pages.providersPage.status.paused',
		bannerKey: 'ui.pages.providersPage.banner.paused'
	},
	needsSetup: {
		name: 'needsSetup',
		tone: 'warn',
		labelKey: 'ui.pages.providersPage.status.needsSetup',
		bannerKey: 'ui.pages.providersPage.banner.needsSetup'
	},
	noAccount: {
		name: 'noAccount',
		tone: 'warn',
		labelKey: 'ui.pages.providersPage.status.noAccount',
		bannerKey: 'ui.pages.providersPage.banner.noAccount'
	},
	signInAgain: {
		name: 'signInAgain',
		tone: 'warn',
		labelKey: 'ui.pages.providersPage.status.signInAgain',
		bannerKey: 'ui.pages.providersPage.banner.signInAgain'
	},
	accountsPaused: {
		name: 'accountsPaused',
		tone: 'warn',
		labelKey: 'ui.pages.providersPage.status.accountsPaused',
		bannerKey: 'ui.pages.providersPage.banner.accountsPaused'
	},
	modelListFailed: {
		name: 'modelListFailed',
		tone: 'warn',
		labelKey: 'ui.pages.providersPage.status.modelListFailed',
		bannerKey: 'ui.pages.providersPage.banner.modelListFailed'
	},
	noModelsOn: {
		name: 'noModelsOn',
		tone: 'warn',
		labelKey: 'ui.pages.providersPage.status.noModelsOn',
		bannerKey: 'ui.pages.providersPage.banner.noModelsOn'
	}
};

/** Names the state one provider is in. */
export function providerStatus(provider: Provider): ProviderStatus {
	if (!provider.enabled) return STATUS.paused;
	if ((provider.needs_setup ?? []).length > 0) return STATUS.needsSetup;
	if (needsCredential(provider) && provider.counts.accounts === 0) return STATUS.noAccount;
	if (provider.counts.reauth_accounts > 0 && provider.counts.active_accounts === 0) {
		return STATUS.signInAgain;
	}
	if (
		provider.counts.accounts > 0 &&
		provider.counts.active_accounts === 0 &&
		provider.counts.paused_accounts > 0
	) {
		return STATUS.accountsPaused;
	}
	if ((provider.last_refresh_error ?? '') !== '') return STATUS.modelListFailed;
	if (provider.counts.models === 0 || provider.counts.enabled_models === 0)
		return STATUS.noModelsOn;
	return STATUS.ready;
}

/** Reports whether a provider cannot route without one, which is what makes an empty account
 *  list worth warning about. */
export function needsCredential(provider: Provider): boolean {
	return provider.auth !== 'none';
}

/** Reports whether an OAuth account has to sign in again. Only the account's own
 *  status says so: a model list that failed to refresh, a timeout or a refused
 *  connection, leaves the account signed in. */
export function accountNeedsSignIn(account: Account): boolean {
	return account.kind === 'oauth' && account.status === 'needs_reauth';
}

/** Names the one action a banner offers, so a banner always has a way out rather than only a
 *  complaint. */
export function bannerAction(name: ProviderStatusName): string {
	switch (name) {
		case 'paused':
			return 'ui.pages.providersPage.banner.actionResume';
		case 'needsSetup':
			return 'ui.pages.providersPage.banner.actionOpenSettings';
		case 'noAccount':
		case 'signInAgain':
			return 'ui.pages.providersPage.banner.actionAddAccount';
		case 'accountsPaused':
			return 'ui.pages.providersPage.banner.actionOpenAccounts';
		case 'modelListFailed':
			return 'ui.pages.providersPage.banner.actionRefresh';
		case 'noModelsOn':
			return 'ui.pages.providersPage.banner.actionShowModels';
		default:
			return '';
	}
}
