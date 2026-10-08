import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';

import Button from './button.svelte';

// A control that renders nothing is a total UI outage that no type checker
// sees: a block tag missing its brace is still valid markup, so the branch
// after it is read as text and the element never reaches the page. These
// assertions render the real component and read the markup it produced.

function markup(props: Record<string, unknown>): string {
	return render(Button, { props }).body;
}

describe('button rendering', () => {
	it('renders one button and never an anchor', () => {
		const html = markup({ children: undefined });
		expect(html.match(/<button/g) ?? []).toHaveLength(1);
		expect(html).toContain('data-slot="button"');
		expect(html).not.toContain('<a ');
	});

	it('keeps a caller class alongside the variant classes', () => {
		const html = markup({ class: 'md:hidden' });
		expect(html).toContain('md:hidden');
		expect(html).toContain('inline-flex');
	});

	it('never leaks a block tag into the rendered text', () => {
		for (const props of [{}, { class: 'gap-2' }, { variant: 'outline' }]) {
			expect(markup(props)).not.toContain(':else');
		}
	});

	it('renders exactly one control for every shape it is given', () => {
		for (const props of [
			{},
			{ class: 'gap-2' },
			{ variant: 'ghost', size: 'icon' },
			{ variant: 'chip' }
		]) {
			const html = markup(props);
			const controls = html.match(/<(button|a) /g) ?? [];
			expect(controls, JSON.stringify(props)).toHaveLength(1);
		}
	});
});
