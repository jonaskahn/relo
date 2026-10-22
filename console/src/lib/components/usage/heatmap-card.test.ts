import { render } from 'svelte/server';
import { addMessages, init } from 'svelte-i18n';
import { beforeAll, describe, expect, it } from 'vitest';

import en from '$lib/i18n/locales/en.json';
import { heatmapCells } from '$lib/usage-charts';

import HeatmapCard from './heatmap-card.svelte';

beforeAll(() => {
	addMessages('en', en);
	init({ fallbackLocale: 'en', initialLocale: 'en' });
});

describe('the activity calendar', () => {
	it('prints the day of the month in every heat cell', () => {
		const weeks = heatmapCells([
			{ key: '2026-09-30', value: 4 },
			{ key: '2026-10-01', value: 8 },
			{ key: '2026-10-04', value: 2 }
		]);
		const html = render(HeatmapCard, { props: { weeks } }).body;
		expect(html).toContain('>30</span>');
		expect(html).toContain('>1</span>');
		expect(html).toContain('>4</span>');
		expect(html).toContain('var(--accent)');
		expect(html).toContain('grid-cols-7');
		for (const weekday of ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun']) {
			expect(html).toContain('>' + weekday + '</span>');
		}
	});
});
