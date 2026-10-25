import { describe, expect, test } from 'vitest';

import {
	SECTION_ORDER,
	addedTemplateIds,
	buildTemplateRows,
	countTemplateRows,
	groupProviders,
	isProviderId,
	loginHint,
	matchesQuery,
	sectionOf,
	suggestLabel,
	suggestProviderId,
	templateHintKey,
	templateHintValue,
	visibleTemplateRows
} from './provider-sections';
import { providerOf, signInTemplate, templateOf } from './provider-fixtures';

describe('provider sections', () => {
	test('lists the sections account, cloud, local, then the long key list', () => {
		expect(SECTION_ORDER).toEqual(['account', 'cloud', 'local', 'key']);
	});

	test('files a kind under its section, and an unknown kind under keys', () => {
		expect(sectionOf('signin')).toBe('account');
		expect(sectionOf('cloud')).toBe('cloud');
		expect(sectionOf('local')).toBe('local');
		expect(sectionOf('key')).toBe('key');
		expect(sectionOf('something-new')).toBe('key');
	});

	test('offers Google Antigravity under the account section', () => {
		const templates = [
			signInTemplate({ id: 'google-antigravity', label: 'Google Antigravity' }),
			signInTemplate()
		];
		expect(buildTemplateRows(templates, new Set()).account.map((row) => row.id)).toEqual([
			'google-antigravity',
			'claude'
		]);
	});

	test('moves a row Relo cannot speak to the end of its section', () => {
		const rows = buildTemplateRows(
			[
				templateOf({ id: 'a-off', label: 'A Off', unsupported_reason: 'needs the x transport' }),
				templateOf({ id: 'b-on', label: 'B On' })
			],
			new Set()
		);
		expect(rows.key.map((row) => row.id)).toEqual(['b-on', 'a-off']);
	});

	test('keeps the daemon order inside a section', () => {
		const rows = buildTemplateRows(
			[templateOf({ id: 'z', label: 'Z' }), templateOf({ id: 'a', label: 'A' })],
			new Set()
		);
		expect(rows.key.map((row) => row.id)).toEqual(['z', 'a']);
	});

	test('closes Local with the custom endpoint card', () => {
		const rows = buildTemplateRows(
			[templateOf({ id: 'ollama', label: 'Ollama', kind: 'local' })],
			new Set()
		);
		expect(rows.local.map((row) => row.id)).toEqual(['ollama', 'custom']);
		expect(rows.local[1].custom).toBe(true);
	});

	test('a search hides the sections it empties', () => {
		const templates = [
			signInTemplate(),
			templateOf({ id: 'openai', label: 'OpenAI' }),
			templateOf({ id: 'amazon-bedrock', label: 'Amazon Bedrock', kind: 'cloud' })
		];
		const rows = buildTemplateRows(templates, new Set(), 'bed');
		const visible = visibleTemplateRows(rows);
		expect(visible.map((group) => group.section)).toEqual(['cloud']);
		expect(countTemplateRows(rows)).toBe(1);
	});

	test('a search that matches nothing still offers the custom endpoint', () => {
		const rows = buildTemplateRows(
			[templateOf({ id: 'openai', label: 'OpenAI' })],
			new Set(),
			'zzz'
		);
		expect(visibleTemplateRows(rows)).toEqual([]);
		const customTyped = buildTemplateRows(
			[templateOf({ id: 'openai', label: 'OpenAI' })],
			new Set(),
			'endpoint'
		);
		expect(visibleTemplateRows(customTyped).map((group) => group.section)).toEqual(['local']);
	});

	test('matches a search on the label and on the id', () => {
		expect(matchesQuery('open', 'openai', 'OpenAI')).toBe(true);
		expect(matchesQuery('OPENAI', 'openai', 'OpenAI')).toBe(true);
		expect(matchesQuery('bedrock', 'amazon-bedrock', 'Amazon Bedrock')).toBe(true);
		expect(matchesQuery('groq', 'openai', 'OpenAI')).toBe(false);
		expect(matchesQuery('', 'openai', 'OpenAI')).toBe(true);
	});

	test('names the sign-in method a row uses', () => {
		expect(loginHint(signInTemplate())).toBe('browser');
		expect(loginHint(signInTemplate({ login_methods: [{ flow: 'x', kind: 'device' }] }))).toBe(
			'device'
		);
		expect(loginHint(signInTemplate({ login_methods: [{ flow: 'x', kind: 'cli' }] }))).toBe('cli');
		expect(
			loginHint(
				signInTemplate({
					login_methods: [
						{ flow: 'x', kind: 'browser' },
						{ flow: 'y', kind: 'device' }
					]
				})
			)
		).toBe('browserOrDevice');
		expect(
			loginHint(
				signInTemplate({
					login_methods: [
						{ flow: 'x', kind: 'browser' },
						{ flow: 'y', kind: 'cli' }
					]
				})
			)
		).toBe('mixed');
	});

	test('names the credentials a cloud row takes', () => {
		expect(templateHintKey(templateOf({ id: 'amazon-bedrock', kind: 'cloud' }))).toBe(
			'ui.pages.providersPage.add.credentialBedrock'
		);
		expect(templateHintKey(templateOf({ id: 'amazon-bedrock-2', kind: 'cloud' }))).toBe(
			'ui.pages.providersPage.add.credentialCloud'
		);
		expect(templateHintKey(templateOf({ id: 'azure-openai', kind: 'cloud' }))).toBe(
			'ui.pages.providersPage.add.credentialAzure'
		);
		expect(templateHintKey(templateOf({ id: 'google-vertex', kind: 'cloud' }))).toBe(
			'ui.pages.providersPage.add.credentialVertex'
		);
	});

	test('names the credentials a key row takes', () => {
		expect(templateHintKey(templateOf({ id: 'openai' }))).toBe(
			'ui.pages.providersPage.add.hintKey'
		);
	});

	test('names a local row by the address it listens on', () => {
		expect(
			templateHintKey(
				templateOf({
					id: 'ollama',
					kind: 'local',
					default_base_url: 'http://127.0.0.1:11434'
				})
			)
		).toBe('');
		expect(
			templateHintValue(
				templateOf({
					id: 'ollama',
					kind: 'local',
					available_formats: [],
					default_base_url: 'http://127.0.0.1:11434'
				})
			)
		).toBe('http://127.0.0.1:11434');
		// A local row with a format choice names the address that choice listens on.
		expect(
			templateHintValue(
				templateOf({
					id: 'ollama',
					kind: 'local',
					default_format: 'openai-chat',
					default_base_url: 'http://unused',
					available_formats: [
						{
							format: 'openai-chat',
							label: 'OpenAI',
							default_base_url: 'http://127.0.0.1:11434/v1',
							key_header: 'bearer',
							models_format: 'openai'
						}
					]
				})
			)
		).toBe('http://127.0.0.1:11434/v1');
		// Only a local row shows an address as its hint value.
		expect(templateHintValue(templateOf({ id: 'openai' }))).toBe('');
	});

	test('counts a quiet section as no rows', () => {
		const rows = buildTemplateRows([templateOf({ id: 'openai' })], new Set());
		expect(rows.account).toEqual([]);
		expect(rows.cloud).toEqual([]);
		expect(countTemplateRows(rows)).toBe(2);
	});

	test('adds a badge for a template that already has a provider', () => {
		const added = addedTemplateIds([providerOf({ id: 'openai-2', template_id: 'openai' })]);
		expect(added.has('openai')).toBe(true);
		expect(added.has('openai-2')).toBe(true);
	});

	test('groups added providers under their section, A to Z', () => {
		const groups = groupProviders([
			providerOf({ id: 'z', label: 'Zed', kind: 'key' }),
			providerOf({ id: 'a', label: 'Alpha', kind: 'key' }),
			providerOf({ id: 'claude', label: 'Claude', kind: 'signin' }),
			providerOf({ id: 'endpoint', label: 'Endpoint', kind: 'local' })
		]);
		expect(groups.map((group) => group.section)).toEqual(['account', 'local', 'key']);
		expect(groups[2].providers.map((provider) => provider.label)).toEqual(['Alpha', 'Zed']);
	});

	test('a search narrows the added providers', () => {
		const groups = groupProviders(
			[providerOf({ id: 'openai', label: 'OpenAI' }), providerOf({ id: 'groq', label: 'Groq' })],
			'gro'
		);
		expect(groups[0].providers.map((provider) => provider.id)).toEqual(['groq']);
	});

	test('suggests a free provider id', () => {
		expect(suggestProviderId('openai', [])).toBe('openai');
		expect(suggestProviderId('openai', ['openai'])).toBe('openai-2');
		expect(suggestProviderId('openai', ['openai', 'openai-2'])).toBe('openai-3');
	});

	test('suggests a free label', () => {
		expect(suggestLabel('OpenAI', [])).toBe('OpenAI');
		expect(suggestLabel('OpenAI', ['OpenAI'])).toBe('OpenAI 2');
	});

	test('accepts only the ids the daemon accepts', () => {
		expect(isProviderId('openai-2')).toBe(true);
		expect(isProviderId('my.endpoint_1')).toBe(true);
		expect(isProviderId('OpenAI')).toBe(false);
		expect(isProviderId('-leading')).toBe(false);
		expect(isProviderId('')).toBe(false);
	});
});

test('files a Command Code key template under keys', () => {
	const rows = buildTemplateRows(
		[templateOf({ id: 'command-code', label: 'Command Code', kind: 'key' })],
		new Set()
	);
	expect(rows.key.map((row) => row.id)).toEqual(['command-code']);
});
