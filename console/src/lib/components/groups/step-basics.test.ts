import { render } from 'svelte/server';
import { describe, expect, it, vi } from 'vitest';

import { initI18n } from '$lib/i18n/index';
import { emptyRouteDraft, type RouteDraft } from '$lib/route-stepper';

import StepBasics from './step-basics.svelte';

// The name step hands Field an already-translated error, the same contract
// the members step keeps through $t(problems.members). Rendering through
// the real catalog proves the operator reads "Name the group first" rather
// than the key that names it.
function markup(problems: Record<string, string>, draft: RouteDraft = emptyRouteDraft()): string {
	return render(StepBasics, {
		props: { draft, creating: true, problems, ondraft: () => {} }
	}).body;
}

describe('basics error message', () => {
	it('renders the translated message instead of the key', async () => {
		vi.stubGlobal('document', { documentElement: { lang: '' } });
		try {
			await initI18n('en');
			const html = markup({ id: 'ui.pages.groupsPage.needId' });
			expect(html).toContain('Name the group first');
			expect(html).not.toContain('ui.pages.groupsPage.needId');
		} finally {
			vi.unstubAllGlobals();
		}
	});

	it('renders no error when the step is clean', async () => {
		vi.stubGlobal('document', { documentElement: { lang: '' } });
		try {
			await initI18n('en');
			expect(markup({})).not.toContain('field-error');
		} finally {
			vi.unstubAllGlobals();
		}
	});
});

describe('basics failover switches', () => {
	it('offers both switches on the first step, read from the draft', async () => {
		vi.stubGlobal('document', { documentElement: { lang: '' } });
		try {
			await initI18n('en');
			const on = markup({});
			expect(on).toContain('Allow switch on 4xx');
			expect(on).toContain('Allow switch on 5xx');
			const off = markup({}, { ...emptyRouteDraft(), switchOn4xx: false });
			expect(off).not.toEqual(on);
			expect(on.match(/aria-checked="true"/g)?.length).toBeGreaterThan(
				off.match(/aria-checked="true"/g)?.length ?? 0
			);
		} finally {
			vi.unstubAllGlobals();
		}
	});
});
