import { render } from 'svelte/server';
import { addMessages, init } from 'svelte-i18n';
import { beforeAll, describe, expect, it } from 'vitest';

import en from '$lib/i18n/locales/en.json';
import { usageRowOf } from '$lib/usage-fixtures';

import RequestReportAction from './request-report-action.svelte';

beforeAll(() => {
	addMessages('en', en);
	init({ fallbackLocale: 'en', initialLocale: 'en' });
});

function markup(status: number) {
	const row = usageRowOf({ Status: status });
	return render(RequestReportAction, { props: { row } }).body;
}

describe('the failed request report action', () => {
	it('appears only when the request failed', () => {
		expect(markup(429)).toContain('data-report-issue-button');
		expect(markup(200)).not.toContain('data-report-issue-button');
	});
});
