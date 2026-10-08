import { render } from 'svelte/server';
import type { ComponentProps } from 'svelte';
import { describe, expect, it } from 'vitest';

import AddTile from './add-tile.svelte';

// The add tile is one button filling its grid cell: a render that loses its
// brace or its label leaves a silent hole where the page's creation action
// should be, so the markup is read here the way the page will show it.

function markup(props: ComponentProps<typeof AddTile>): string {
	return render(AddTile, { props }).body;
}

const base = {
	label: 'Add connection',
	hint: 'Account, API key or endpoint',
	onclick: () => {}
};

describe('add tile', () => {
	it('renders exactly one control carrying the label and the hint', () => {
		const html = markup(base);
		expect(html.match(/<button/g) ?? []).toHaveLength(1);
		expect(html).toContain('data-plain');
		expect(html).toContain('Add connection');
		expect(html).toContain('Account, API key or endpoint');
	});

	it('wears the dashed accent outline and the plus circle', () => {
		const html = markup(base);
		expect(html).toContain('border-dashed');
		expect(html).toContain('border-accent-border');
		expect(html).toContain('rounded-full');
	});

	it('keeps a caller class beside its own', () => {
		const html = markup({ ...base, class: 'md:hidden' });
		expect(html).toContain('md:hidden');
		expect(html).toContain('border-dashed');
	});

	it('never leaks a block tag into the rendered text', () => {
		expect(markup(base)).not.toContain(':else');
	});
});
