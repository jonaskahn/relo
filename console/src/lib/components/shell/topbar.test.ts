import { render } from 'svelte/server';
import { addMessages, init } from 'svelte-i18n';
import { beforeAll, describe, expect, it } from 'vitest';

import en from '$lib/i18n/locales/en.json';

import Topbar from './topbar.svelte';

beforeAll(() => {
	addMessages('en', en);
	init({ fallbackLocale: 'en', initialLocale: 'en' });
});

describe('the top bar issue control', () => {
	it('is labelled, carries the GitHub icon, and names itself for assistive technology', () => {
		const html = render(Topbar).body;
		expect(html).toContain('data-report-issue-button');
		expect(html).toContain('Report issue');
		expect(html).toContain('aria-label="Report issue"');
		expect(html).toContain('title="Open a Relo issue on GitHub"');
	});
});
