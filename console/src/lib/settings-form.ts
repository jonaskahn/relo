import type { Provider, TemplateFormatOption } from '$lib/types';
import { cloneWindows } from '$lib/upstream-settings';

/** The connection form before it is saved. */
export interface ConnectionDraft {
	label: string;
	enabled: boolean;
	rank: string;
	poolStrategy: string;
	useProxy: boolean;
	// timeoutSeconds and retryBackoff are the connection's own provider waits;
	// null means the connection uses the global value.
	timeoutSeconds: number | null;
	retryBackoff: [number, number][] | null;
	// switchOn4xx and switchOn5xx move a request to another account of this
	// connection after a status the relay does not always retry.
	switchOn4xx: boolean;
	switchOn5xx: boolean;
	apiFormat: string;
	baseURL: string;
	variables: Record<string, string>;
	headers: Record<string, string>;
	keyHeader: string;
}

/** The sign-in choices stored beside the connection, for a template that stores them: the token
 *  renewal every sign-in has. Whether a model is published twice, at 200K and at 1M, follows from
 *  the model itself, so no stored choice governs it. */
export interface SignInDraft {
	autoRefresh: boolean;
}

/** The settings section one sign-in template shows. */
export interface SignInSection {
	title: string;
	description: string;
}

/** Names the section a connection's settings tab shows for its sign-in template, or null for a
 *  template that stores no sign-in choices. */
export function signInSection(templateID: string): SignInSection | null {
	switch (templateID) {
		case 'claude':
			return {
				title: 'ui.pages.providersPage.settings.claudeTitle',
				description: 'ui.pages.providersPage.settings.claudeDescription'
			};
		case 'openai-codex':
			return {
				title: 'ui.pages.providersPage.settings.chatgptTitle',
				description: 'ui.pages.providersPage.settings.chatgptDescription'
			};
		default:
			return null;
	}
}

/** Reports whether the endpoint disclosure should be open on first paint: custom connections, or
 *  a saved value that is not the default. */
export function startsAdvanced(provider: Provider, formats: TemplateFormatOption[]): boolean {
	if (provider.origin === 'custom') return true;
	const selected = formats.find((option) => option.format === provider.api_format);
	if (selected && provider.base_url !== selected.default_base_url) return true;
	if (Object.keys(provider.headers ?? {}).length > 0) return true;
	return Object.values(provider.variables ?? {}).some((value) => value !== '');
}

/** A connection's editable fields as the settings form holds them. */
export function connectionDraft(provider: Provider): ConnectionDraft {
	return {
		label: provider.label,
		enabled: provider.enabled,
		rank: String(provider.rank),
		poolStrategy: provider.pool_strategy,
		useProxy: provider.use_proxy,
		timeoutSeconds: provider.timeout_seconds ?? null,
		retryBackoff: provider.retry_backoff ? cloneWindows(provider.retry_backoff) : null,
		switchOn4xx: provider.switch_on_4xx,
		switchOn5xx: provider.switch_on_5xx,
		apiFormat: provider.api_format,
		baseURL: provider.base_url,
		variables: { ...provider.variables },
		headers: { ...provider.headers },
		keyHeader: provider.key_header
	};
}

/** Reports whether the form still differs from the stored connection. */
export function connectionDirty(draft: ConnectionDraft, provider: Provider): boolean {
	const current = connectionDraft(provider);
	return (
		draft.label !== current.label ||
		draft.enabled !== current.enabled ||
		draft.rank !== current.rank ||
		draft.poolStrategy !== current.poolStrategy ||
		draft.useProxy !== current.useProxy ||
		draft.timeoutSeconds !== current.timeoutSeconds ||
		JSON.stringify(draft.retryBackoff) !== JSON.stringify(current.retryBackoff) ||
		draft.switchOn4xx !== current.switchOn4xx ||
		draft.switchOn5xx !== current.switchOn5xx ||
		draft.apiFormat !== current.apiFormat ||
		draft.baseURL !== current.baseURL ||
		JSON.stringify(draft.variables) !== JSON.stringify(current.variables) ||
		JSON.stringify(draft.headers) !== JSON.stringify(current.headers) ||
		draft.keyHeader !== current.keyHeader
	);
}

/** Reports whether the sign-in fields still differ from what is stored. */
export function signInDirty(draft: SignInDraft, saved: SignInDraft): boolean {
	return draft.autoRefresh !== saved.autoRefresh;
}

/** Reports a rank that is not a whole number. */
export function rankInvalid(rank: string): boolean {
	return !/^-?\d+$/.test(rank.trim());
}

/** Reports an empty endpoint. */
export function baseURLMissing(baseURL: string): boolean {
	return baseURL.trim() === '';
}

/** Names only the sign-in choices that changed. */
export function templatePatch(
	draft: SignInDraft,
	saved: SignInDraft
): { auto_refresh?: boolean } | null {
	const patch: { auto_refresh?: boolean } = {};
	if (draft.autoRefresh !== saved.autoRefresh) patch.auto_refresh = draft.autoRefresh;
	return Object.keys(patch).length === 0 ? null : patch;
}
