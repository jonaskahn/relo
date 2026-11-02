import { render } from 'svelte/server';
import { addMessages, init } from 'svelte-i18n';
import { beforeAll, describe, expect, it } from 'vitest';

import en from '$lib/i18n/locales/en.json';
import { providerOf } from '$lib/provider-fixtures';

import ProviderPane from './provider-pane.svelte';

beforeAll(() => {
	addMessages('en', en);
	init({ fallbackLocale: 'en', initialLocale: 'en' });
});

function markup(provider: ReturnType<typeof providerOf>) {
	return render(ProviderPane, {
		props: {
			provider,
			formats: [],
			onback: () => {},
			onchanged: () => {},
			ondeleted: () => {},
			ontab: () => {}
		}
	}).body;
}

describe('the connection pane dashboard chip', () => {
	it('opens the curated dashboard in a new tab', () => {
		const html = markup(
			providerOf({
				id: 'open-design',
				template_id: 'open-design',
				label: 'OpenDesign',
				dashboard_url: 'https://open-design.ai/cloud/dashboard'
			})
		);
		expect(html).toContain('href="https://open-design.ai/cloud/dashboard"');
		expect(html).toContain('target="_blank"');
		expect(html).toContain('rel="noopener noreferrer"');
		expect(html).toContain('Open dashboard for OpenDesign');
	});

	it('omits the chip when no dashboard is curated', () => {
		const html = markup(providerOf({ id: 'ollama', template_id: 'ollama', label: 'Ollama' }));
		expect(html).not.toContain('data-dashboard-link');
	});
});
