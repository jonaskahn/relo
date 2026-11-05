import { render } from 'svelte/server';
import { addMessages, init } from 'svelte-i18n';
import { beforeAll, describe, expect, it } from 'vitest';

import en from '$lib/i18n/locales/en.json';

import Topbar from './topbar.svelte';

beforeAll(() => {
	addMessages('en', en);
	init({ fallbackLocale: 'en', initialLocale: 'en' });
});

describe('the top bar support control', () => {
	it('is icon-only and accessible at every breakpoint', () => {
		const html = render(Topbar).body;
		expect(html).toContain('aria-label="Support"');
		expect(html).toContain('title="Support"');
		expect(html).not.toContain('data-report-issue-button');
		expect(html).not.toContain('Report issue');
		expect(html).not.toMatch(/>\s*Support\s*</);
	});
});
