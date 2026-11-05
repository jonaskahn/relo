import { render } from 'svelte/server';
import { readFileSync } from 'node:fs';
import { addMessages, init } from 'svelte-i18n';
import { beforeAll, describe, expect, it } from 'vitest';

import en from '$lib/i18n/locales/en.json';
import { usageRowOf } from '$lib/usage-fixtures';

import ProviderErrorReport from './provider-error-report.svelte';
import { hasProviderError } from '$lib/provider-report';

beforeAll(() => {
	addMessages('en', en);
	init({ fallbackLocale: 'en', initialLocale: 'en' });
});

function markup(status: number) {
	const row = usageRowOf({ Status: status });
	return render(ProviderErrorReport, { props: { row } }).body;
}

describe('the failed request report action', () => {
	it('uses provider captures, not the final status, to gate the action', () => {
		const source = readFileSync(new URL('./request-detail-modal.svelte', import.meta.url), 'utf8');
		expect(source).toContain('!capturesLoading && hasProviderError(captures)');
		expect(hasProviderError([])).toBe(false);
		expect(markup(429)).toContain('Report provider error');
	});

	it('renders the request detail as a trailing drawer', () => {
		const source = readFileSync(new URL('./request-detail-modal.svelte', import.meta.url), 'utf8');
		expect(source).toContain('placement="drawer"');
		expect(source).toContain('<RequestDetailOverview {row} />');
		expect(source).not.toContain('request body');
	});
});
