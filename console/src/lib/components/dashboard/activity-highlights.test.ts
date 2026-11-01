import { render } from 'svelte/server';
import { addMessages, init } from 'svelte-i18n';
import { beforeAll, describe, expect, it } from 'vitest';

import en from '$lib/i18n/locales/en.json';

import ActivityHighlights from './activity-highlights.svelte';

beforeAll(() => {
	addMessages('en', en);
	init({ fallbackLocale: 'en', initialLocale: 'en' });
});

describe('the dashboard activity highlights', () => {
	it('names all three figures and the dates they refer to', () => {
		const html = render(ActivityHighlights, {
			props: {
				busiest: { key: '2026-10-23', value: 5046 },
				longest: { days: 16, start: '2026-10-01', end: '2026-10-16' },
				current: { days: 14, start: '2026-10-19', end: '2026-11-01' },
				loading: false,
				pending: undefined as never
			}
		}).body;
		expect(html).toContain('Busiest day');
		expect(html).toContain('Longest streak');
		expect(html).toContain('Current streak');
		expect(html).toContain('5.0k');
		expect(html).toContain('Oct 23');
		expect(html).toContain('16');
		expect(html).toContain('Oct 1 – Oct 16');
		expect(html).toContain('since Oct 19');
		expect(html).toContain('requests');
		expect(html).toContain('days');
		expect(html).toContain('section-surface');
	});

	it('names a one-day run once', () => {
		const html = render(ActivityHighlights, {
			props: {
				busiest: { key: '2026-10-23', value: 5 },
				longest: { days: 1, start: '2026-10-23', end: '2026-10-23' },
				current: null,
				loading: false,
				pending: undefined as never
			}
		}).body;
		expect(html).toContain('Oct 23');
		expect(html).not.toContain('Oct 23 – Oct 23');
		expect(html).toContain('No active streak');
	});

	it('says there is nothing to show rather than inventing a year', () => {
		const html = render(ActivityHighlights, {
			props: {
				busiest: null,
				longest: null,
				current: null,
				loading: false,
				pending: undefined as never
			}
		}).body;
		expect(html).toContain('—');
		expect(html).toContain('No data yet.');
		expect(html).toContain('No active streak');
	});
});
