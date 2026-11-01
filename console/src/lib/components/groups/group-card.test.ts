import { render } from 'svelte/server';
import { addMessages, init } from 'svelte-i18n';
import { beforeAll, describe, expect, it } from 'vitest';

import en from '$lib/i18n/locales/en.json';
import type { Group } from '$lib/types';

import GroupCard from './group-card.svelte';

beforeAll(() => {
	addMessages('en', en);
	init({ fallbackLocale: 'en', initialLocale: 'en' });
});

const group: Group = {
	id: 'fast',
	label: 'Fast',
	strategy: 'priority',
	enabled: true,
	listed: true,
	switch_on_4xx: true,
	switch_on_5xx: true,
	shadows_model: false,
	members: [],
	created_at_ms: 1,
	updated_at_ms: 1
};

describe('the group routing inspector action', () => {
	it('is the first footer action and names what it opens', () => {
		const html = render(GroupCard, {
			props: {
				group,
				providers: [],
				onopen: () => {},
				onedit: () => {},
				onremove: () => {},
				oninspect: () => {}
			}
		}).body;
		expect(html).toContain('aria-label="Inspect routing"');
		expect(html.indexOf('Inspect routing')).toBeLessThan(html.indexOf('aria-label="Edit group"'));
	});
});
