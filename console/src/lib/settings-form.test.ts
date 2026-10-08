import { describe, expect, it } from 'vitest';

import { providerOf } from '$lib/provider-fixtures';
import type { TemplateFormatOption } from '$lib/types';

import {
	baseURLMissing,
	connectionDirty,
	connectionDraft,
	rankInvalid,
	signInDirty,
	signInSection,
	startsAdvanced,
	templatePatch
} from './settings-form';

const chat: TemplateFormatOption = {
	format: 'openai-chat',
	default_base_url: 'https://api.openai.com/v1',
	key_header: 'bearer',
	models_format: 'openai',
	label: 'Chat'
};

describe('connection settings draft', () => {
	it('is dirty only when a connection field changed', () => {
		const provider = providerOf();
		const draft = connectionDraft(provider);

		expect(connectionDirty(draft, provider)).toBe(false);
		expect(connectionDirty({ ...draft, label: 'Other' }, provider)).toBe(true);
	});

	it('tracks the per-connection provider waits', () => {
		const provider = providerOf();
		const draft = connectionDraft(provider);
		expect(draft.timeoutSeconds).toBeNull();
		expect(draft.retryBackoff).toBeNull();
		expect(connectionDirty({ ...draft, timeoutSeconds: 30 }, provider)).toBe(true);
		expect(
			connectionDirty(
				{
					...draft,
					retryBackoff: [
						[2, 4],
						[4, 6],
						[6, 8]
					]
				},
				provider
			)
		).toBe(true);

		const overridden = providerOf({
			timeout_seconds: 30,
			retry_backoff: [
				[2, 4],
				[4, 6],
				[6, 8]
			]
		});
		const overriddenDraft = connectionDraft(overridden);
		expect(overriddenDraft.timeoutSeconds).toBe(30);
		expect(overriddenDraft.retryBackoff).toEqual([
			[2, 4],
			[4, 6],
			[6, 8]
		]);
		expect(connectionDirty(overriddenDraft, overridden)).toBe(false);
	});

	it('patches only the sign-in choice that changed', () => {
		const saved = { autoRefresh: true };

		expect(signInDirty(saved, saved)).toBe(false);
		expect(templatePatch(saved, saved)).toBeNull();
		expect(templatePatch({ autoRefresh: false }, saved)).toEqual({ auto_refresh: false });
	});

	it('names the sign-in section a template shows', () => {
		expect(signInSection('claude')?.title).toBe('ui.pages.providersPage.settings.claudeTitle');
		expect(signInSection('openai-codex')?.title).toBe(
			'ui.pages.providersPage.settings.chatgptTitle'
		);
		expect(signInSection('openai')).toBeNull();
	});

	it('rejects a blank endpoint and a rank that is not a whole number', () => {
		expect(rankInvalid('10')).toBe(false);
		expect(rankInvalid('-2')).toBe(false);
		expect(rankInvalid('1.5')).toBe(true);
		expect(rankInvalid('')).toBe(true);
		expect(baseURLMissing(' https://example.test ')).toBe(false);
		expect(baseURLMissing('  ')).toBe(true);
	});

	it('opens advanced for a custom connection or a value that is not the default', () => {
		expect(startsAdvanced(providerOf(), [chat])).toBe(false);
		expect(startsAdvanced(providerOf({ origin: 'custom' }), [chat])).toBe(true);
		expect(startsAdvanced(providerOf({ base_url: 'https://other.example/v1' }), [chat])).toBe(true);
		expect(startsAdvanced(providerOf({ headers: { 'X-Trace': '1' } }), [chat])).toBe(true);
	});
});
