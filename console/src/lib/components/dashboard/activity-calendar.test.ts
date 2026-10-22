import { render } from 'svelte/server';
import { addMessages, init } from 'svelte-i18n';
import { beforeAll, describe, expect, it } from 'vitest';

import en from '$lib/i18n/locales/en.json';
import { heatmapCells } from '$lib/usage-charts';

import ActivityCalendar from './activity-calendar.svelte';

beforeAll(() => {
	addMessages('en', en);
	init({ fallbackLocale: 'en', initialLocale: 'en' });
});

function pending() {
	return '';
}

describe('the dashboard activity calendar', () => {
	it('draws one heat cell per day and names the months', () => {
		const weeks = heatmapCells([
			{ key: '2026-09-30', value: 4 },
			{ key: '2026-10-01', value: 8 },
			{ key: '2026-10-04', value: 2 }
		]);
		const html = render(ActivityCalendar, {
			props: { weeks, loading: false, pending, today: '2026-10-04' }
		}).body;
		expect(html).toContain('Activity');
		expect(html).toContain('Sep');
		expect(html).toContain('Oct');
		expect(html).toContain('max-sm:hidden');
		expect(html).toContain('h-2.5 w-full');
		expect(html).toContain('var(--accent)');
	});

	it('says there is nothing to draw rather than inventing a year', () => {
		const html = render(ActivityCalendar, {
			props: { weeks: [], loading: false, pending, today: '' }
		}).body;
		expect(html).toContain('No data yet.');
	});
});
