import { render } from 'svelte/server';
import { addMessages, init } from 'svelte-i18n';
import { beforeAll, describe, expect, it } from 'vitest';

import en from '$lib/i18n/locales/en.json';
import { providerOf } from '$lib/provider-fixtures';
import type { Provider, TemplateFormatOption } from '$lib/types';

import SignInSettingsSection from './sign-in-settings-section.svelte';
import SettingsTab from './settings-tab.svelte';

beforeAll(() => {
	addMessages('en', en);
	init({ fallbackLocale: 'en', initialLocale: 'en' });
});

const chat: TemplateFormatOption = {
	format: 'openai-chat',
	default_base_url: 'https://api.openai.com/v1',
	key_header: 'bearer',
	models_format: 'openai',
	label: 'Chat'
};

const responses: TemplateFormatOption = {
	...chat,
	format: 'openai-responses',
	label: 'Responses'
};

function markup(provider: Provider, formats: TemplateFormatOption[] = [chat]): string {
	return render(SettingsTab, {
		props: { provider, allFormats: formats, onchanged: () => {} }
	}).body;
}

describe('connection settings', () => {
	it('stacks general, routing, and an always-open advanced section for a template connection', () => {
		const html = markup(providerOf());

		expect(html).toContain('data-section="general"');
		expect(html).toContain('data-section="routing"');
		expect(html).toContain('data-section="advanced"');
		expect(html).not.toContain('data-section="claude"');
		expect(html).not.toContain('data-advanced-open');
		expect(html).not.toContain('settings-disclosure-toggle');
		expect(html).toContain('Base URL');
		expect(html).toContain('Headers');
		expect(html).not.toContain('Key header');
		expect(html).not.toContain('API format');
		expect(html).not.toContain('data-savebar');
	});

	it('shows the Claude sign-in section while its choices load', () => {
		const html = markup(
			providerOf({ id: 'claude', template_id: 'claude', label: 'Claude', auth: 'oauth' })
		);

		expect(html).toContain('data-section="signin"');
		expect(html.indexOf('data-section="advanced"')).toBeLessThan(
			html.indexOf('data-section="signin"')
		);
		expect(html).toContain('Loading sign-in settings');
		expect(html).toContain('Refresh the token automatically');
		// The 200K companion follows from the model, so there is no switch.
		expect(html).not.toContain('Offer 200K context for 1M models');
	});

	it('shows the ChatGPT sign-in section with token renewal', () => {
		const html = markup(
			providerOf({
				id: 'openai-codex',
				template_id: 'openai-codex',
				label: 'ChatGPT',
				auth: 'oauth'
			})
		);

		expect(html).toContain('data-section="signin"');
		expect(html).toContain('ChatGPT sign-in');
		expect(html).toContain('Refresh the token automatically');
	});

	it('always shows the advanced settings with the key header for a custom connection', () => {
		const html = markup(providerOf({ origin: 'custom' }), [chat, responses]);

		expect(html).not.toContain('data-advanced-open');
		expect(html).not.toContain('settings-disclosure-toggle');
		expect(html).toContain('Key header');
		expect(html).toContain('API format');
	});

	it('always shows the advanced settings when the saved endpoint is not the default', () => {
		const html = markup(providerOf({ base_url: 'https://other.example/v1' }));

		expect(html).not.toContain('data-advanced-open');
		expect(html).toContain('Base URL');
		expect(html).toContain('Reset to default');
	});

	it('shows the provider wait section with the global values inherited', () => {
		const html = markup(providerOf());

		expect(html).toContain('data-section="upstream"');
		expect(html).toContain('Provider waits');
		expect(html).toContain('Use global (5m)');
		expect(html).toContain('Use global (1–3s, 3–5s, 5–10s)');
		expect(html).toContain('Call wait');
	});

	it('shows a retry when Claude settings fail to load and keeps the switches disabled', () => {
		const html = render(SignInSettingsSection, {
			props: {
				status: 'error',
				autoRefresh: true,
				disabled: false,
				onretry: () => {},
				onchange: () => {}
			}
		}).body;

		expect(html).toContain('Sign-in settings could not be loaded.');
		expect(html).toContain('Try again');
		expect(html).toContain('disabled');
	});
});
