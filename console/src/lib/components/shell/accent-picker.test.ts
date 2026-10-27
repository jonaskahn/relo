import { render } from 'svelte/server';
import { addMessages, init } from 'svelte-i18n';
import { beforeAll, describe, expect, it } from 'vitest';

import en from '$lib/i18n/locales/en.json';

import AccentPicker from './accent-picker.svelte';
import ColorRadar from './color-radar.svelte';

beforeAll(() => {
	addMessages('en', en);
	init({ fallbackLocale: 'en', initialLocale: 'en' });
});

function markup(layout: 'page' | 'popover'): string {
	return render(AccentPicker, { props: { layout } }).body;
}

describe('the accent board', () => {
	it('offers the 19 presets and the radar slot, in the order they are listed', () => {
		const html = markup('page');

		expect(html.match(/role="radio"/g) ?? []).toHaveLength(20);
		for (const slot of ['terracotta', 'persimmon', 'matcha', 'graphite', 'custom']) {
			expect(html).toContain(`data-accent="${slot}"`);
		}
		// The accent older builds shipped as red is a colour now, not a slot.
		expect(html).not.toContain('data-accent="red"');
		expect(html).toContain('Custom');
	});

	it('keeps the radar in Settings, where there is room for it', () => {
		expect(markup('page')).not.toContain('id="accent-radar"');
		expect(markup('popover')).toContain('grid-cols-5');
		expect(markup('popover')).not.toContain('id="accent-radar"');
	});
});

describe('the radar', () => {
	it('renders the square, the rail and the colour field on the colour it was given', () => {
		const html = render(ColorRadar, { props: { value: '#c8102e', onchange: () => {} } }).body;

		expect(html).toContain('radar-square');
		expect(html).toContain('radar-rail');
		expect(html.match(/role="slider"/g) ?? []).toHaveLength(2);
		expect(html).toContain('background: #c8102e');
		expect(html).toContain('value="#c8102e"');
	});
});
