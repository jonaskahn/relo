import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';

import TunnelMark from './tunnel-mark.svelte';

describe('the themed provider mark', () => {
	it('draws a path-based mark with the theme ink', () => {
		const html = render(TunnelMark, {
			props: { markId: 'codex', label: 'Codex', x: 10, y: 10, size: 20 }
		}).body;
		expect(html).toContain('var(--ink)');
		expect(html).not.toContain('<image');
	});

	it('inverts a monochrome image mark in dark mode', () => {
		const html = render(TunnelMark, {
			props: { markId: 'openai', label: 'OpenAI', x: 10, y: 10, size: 20 }
		}).body;
		expect(html).toContain('<image');
		expect(html).toContain('dark:invert');
	});

	it('leaves a coloured mark unchanged', () => {
		const html = render(TunnelMark, {
			props: { markId: '302ai', label: '302.AI', x: 10, y: 10, size: 20 }
		}).body;
		expect(html).toContain('<image');
		expect(html).not.toContain('dark:invert');
	});
});
