import type { Provider, ProviderTemplate } from '$lib/types';

// The four sections the console groups providers under, in the order both
// screens list them: the account that signs in, the cloud the provider runs
// on, the server on this machine, then the key from a dashboard. The long
// list of key providers comes last so the short sections stay reachable.

/** The group a connection is filed under: the account it signs into, the cloud it runs on, a
 *  server on this machine, or a key from a dashboard. */
export type ProviderSection = 'account' | 'cloud' | 'local' | 'key';

/** The add-connection list's grouping, in the order it shows. */
export const SECTION_ORDER: ProviderSection[] = ['account', 'cloud', 'local', 'key'];

/** The copy key each section heading reads. */
export const SECTION_LABEL_KEY: Record<ProviderSection, string> = {
	account: 'ui.pages.providersPage.list.groupAccount',
	cloud: 'ui.pages.providersPage.list.groupCloud',
	local: 'ui.pages.providersPage.list.groupLocal',
	key: 'ui.pages.providersPage.list.groupKey'
};

/** Maps the kind a template declares onto the section it is listed under.
 *  An unknown kind is a key provider, which is the common case: a row is either something an
 *  operator signs into, or something they paste a key from. */
export function sectionOf(kind: string): ProviderSection {
	switch (kind) {
		case 'signin':
			return 'account';
		case 'cloud':
			return 'cloud';
		case 'local':
			return 'local';
		default:
			return 'key';
	}
}

/** Names how a sign-in completes, which is what tells an operator whether a browser is about to
 *  open or a code is about to appear. */
export type LoginHint = 'browser' | 'device' | 'cli' | 'browserOrDevice' | 'mixed';

/** The copy key each sign-in hint reads. */
export const LOGIN_HINT_KEY: Record<LoginHint, string> = {
	browser: 'ui.pages.providersPage.add.hintBrowser',
	device: 'ui.pages.providersPage.add.hintDevice',
	cli: 'ui.pages.providersPage.add.hintCli',
	browserOrDevice: 'ui.pages.providersPage.add.hintBrowserOrDevice',
	mixed: 'ui.pages.providersPage.add.hintMixed'
};

/** Which hint a template's sign-in needs. */
export function loginHint(template: ProviderTemplate): LoginHint {
	const kinds = new Set((template.login_methods ?? []).map((method) => method.kind));
	const browser = kinds.has('browser');
	const device = kinds.has('device');
	const cli = kinds.has('cli');
	if (browser && device && !cli && kinds.size === 2) return 'browserOrDevice';
	if (kinds.size <= 1) {
		if (browser) return 'browser';
		if (device) return 'device';
		if (cli) return 'cli';
	}
	return 'mixed';
}

/** Names the credentials a cloud row takes, so a cloud row never reads as one more API key. */
export function credentialHintKey(templateId: string): string {
	switch (templateId) {
		case 'azure-openai':
			return 'ui.pages.providersPage.add.credentialAzure';
		case 'amazon-bedrock':
			return 'ui.pages.providersPage.add.credentialBedrock';
		case 'google-vertex':
			return 'ui.pages.providersPage.add.credentialVertex';
		default:
			return '';
	}
}

/** The row that opens the custom endpoint form.
 *  It is a card at the end of the Local section rather than a section of its own. */
export const CUSTOM_TEMPLATE_ID = 'custom';

/** One row of the add screen, whether it came from the daemon's template list or is the custom
 *  endpoint card. */
export interface TemplateRow {
	id: string;
	label: string;
	section: ProviderSection;
	custom: boolean;
	template: ProviderTemplate | null;
	added: boolean;
	disabledReason: string;
	// hint is a translation key, or an empty string for a row whose hint is a
	// value rather than prose.
	hintKey: string;
	// hintValue carries a value the row shows verbatim, such as the address a
	// local server listens on.
	hintValue: string;
}

/** Names every template an operator already added.
 *  A provider added from a generated template keeps the template id it came from, so the badge
 *  is right even when the row was renamed. */
export function addedTemplateIds(providers: Provider[]): Set<string> {
	const ids = new Set<string>();
	for (const provider of providers) {
		if (provider.template_id) ids.add(provider.template_id);
		ids.add(provider.id);
	}
	return ids;
}

/** Reports whether a row answers a search, which covers the label an operator reads and the id
 *  they type. */
export function matchesQuery(query: string, id: string, label: string): boolean {
	const needle = query.trim().toLowerCase();
	if (needle === '') return true;
	return id.toLowerCase().includes(needle) || label.toLowerCase().includes(needle);
}

// Google Antigravity is left out of the add list while its sign-in is
// unreliable. Drop the id from this set to offer it again.
const pausedTemplateIds = new Set(['google-antigravity']);

/** Keeps the templates the add flow offers, in their sections. */
export function offeredTemplates(templates: readonly ProviderTemplate[]): ProviderTemplate[] {
	return templates.filter((template) => !pausedTemplateIds.has(template.id));
}

/** Groups every template the daemon offers into the four sections, in the daemon's order, with
 *  unsupported rows last inside their section.
 *  A search hides empty sections and always keeps the custom card, which is what an operator
 *  falls back to when nothing matches. */
export function buildTemplateRows(
	templates: readonly ProviderTemplate[],
	added: Set<string>,
	query = ''
): Record<ProviderSection, TemplateRow[]> {
	const rows: Record<ProviderSection, TemplateRow[]> = {
		account: [],
		cloud: [],
		local: [],
		key: []
	};
	const searching = query.trim() !== '';

	for (const template of offeredTemplates(templates)) {
		if (!matchesQuery(query, template.id, template.label)) continue;
		rows[sectionOf(template.kind)].push({
			id: template.id,
			label: template.label,
			section: sectionOf(template.kind),
			custom: false,
			template,
			added: added.has(template.id),
			disabledReason: template.unsupported_reason ?? '',
			hintKey: templateHintKey(template),
			hintValue: templateHintValue(template)
		});
	}

	for (const section of SECTION_ORDER) {
		// A row Relo cannot speak to stays visible and moves to the end of its
		// section, so the reason is readable without hiding the row.
		rows[section] = [
			...rows[section].filter((row) => row.disabledReason === ''),
			...rows[section].filter((row) => row.disabledReason !== '')
		];
	}

	const customLabel = 'Custom endpoint';
	if (!searching || matchesQuery(query, CUSTOM_TEMPLATE_ID, customLabel)) {
		rows.local.push({
			id: CUSTOM_TEMPLATE_ID,
			label: customLabel,
			section: 'local',
			custom: true,
			template: null,
			added: false,
			disabledReason: '',
			hintKey: 'ui.pages.providersPage.add.customEndpointHint',
			hintValue: ''
		});
	}

	return rows;
}

/** Names the line under a template's name: how it signs in, what credentials it takes, or what
 *  it speaks. */
export function templateHintKey(template: ProviderTemplate): string {
	switch (sectionOf(template.kind)) {
		case 'account':
			return LOGIN_HINT_KEY[loginHint(template)];
		case 'cloud': {
			const key = credentialHintKey(template.id);
			return key === '' ? 'ui.pages.providersPage.add.credentialCloud' : key;
		}
		case 'local':
			return '';
		default:
			return 'ui.pages.providersPage.add.hintKey';
	}
}

/** The value a row shows instead of prose, which for a local server is the address it listens
 *  on. */
export function templateHintValue(template: ProviderTemplate): string {
	if (sectionOf(template.kind) !== 'local') return '';
	const option = (template.available_formats ?? []).find(
		(format) => format.format === template.default_format
	);
	return option?.default_base_url ?? template.default_base_url;
}

/** Drops the sections a search left empty, in the order the sections are fixed. */
export function visibleTemplateRows(
	rows: Record<ProviderSection, TemplateRow[]>
): { section: ProviderSection; rows: TemplateRow[] }[] {
	const out: { section: ProviderSection; rows: TemplateRow[] }[] = [];
	for (const section of SECTION_ORDER) {
		if (rows[section].length === 0) continue;
		out.push({ section, rows: rows[section] });
	}
	return out;
}

/** How many rows a grouped template list draws. */
export function countTemplateRows(rows: Record<ProviderSection, TemplateRow[]>): number {
	return SECTION_ORDER.reduce((total, section) => total + rows[section].length, 0);
}

/** The added providers under one heading, sorted by label. */
export interface ProviderGroup {
	section: ProviderSection;
	providers: Provider[];
}

/** Files every added provider under its section, sorted A to Z by the label an operator reads.
 *  Custom endpoints are local, because that is what they are: an address on this machine or the
 *  network it reaches. */
export function groupProviders(providers: Provider[], query = ''): ProviderGroup[] {
	const groups: Record<ProviderSection, Provider[]> = {
		account: [],
		cloud: [],
		local: [],
		key: []
	};
	for (const provider of providers) {
		if (!matchesQuery(query, provider.id, provider.label)) continue;
		groups[sectionOf(provider.kind)].push(provider);
	}
	const out: ProviderGroup[] = [];
	for (const section of SECTION_ORDER) {
		if (groups[section].length === 0) continue;
		groups[section].sort((left, right) => left.label.localeCompare(right.label));
		out.push({ section, providers: groups[section] });
	}
	return out;
}

/** Returns the first free identifier for a template, which is the template id itself until an
 *  operator adds a second one from it. */
export function suggestProviderId(base: string, taken: Iterable<string>): string {
	const used = new Set(taken);
	const trimmed = base.trim() === '' ? 'provider' : base.trim();
	if (!used.has(trimmed)) return trimmed;
	for (let suffix = 2; suffix < 1000; suffix++) {
		const candidate = trimmed + '-' + suffix;
		if (!used.has(candidate)) return candidate;
	}
	return trimmed + '-2';
}

/** Appends a counter to a label an operator already used, so two providers from one template do
 *  not read as the same thing. */
export function suggestLabel(base: string, taken: Iterable<string>): string {
	const used = new Set(taken);
	const trimmed = base.trim();
	if (trimmed === '' || !used.has(trimmed)) return trimmed;
	for (let suffix = 2; suffix < 1000; suffix++) {
		const candidate = trimmed + ' ' + suffix;
		if (!used.has(candidate)) return candidate;
	}
	return trimmed;
}

/** Reports whether a value the operator typed is an identifier the daemon accepts, so Review
 *  refuses one before the request does. */
export function isProviderId(value: string): boolean {
	return /^[a-z0-9][a-z0-9._-]{0,63}$/.test(value.trim());
}
