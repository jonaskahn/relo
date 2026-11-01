import { render } from 'svelte/server';
import { addMessages, init } from 'svelte-i18n';
import { beforeAll, describe, expect, it } from 'vitest';

import en from '$lib/i18n/locales/en.json';
import type { DaemonLine } from '$lib/process-logs';

import ProcessLine from './process-line.svelte';

beforeAll(() => {
	addMessages('en', en);
	init({ fallbackLocale: 'en', initialLocale: 'en' });
});

function line(level: string): DaemonLine {
	return { offset: 1, timestamp_ms: 1, level, message: 'upstream attempt failed' };
}

describe('the daemon log report action', () => {
	it('appears only on an error line', () => {
		expect(render(ProcessLine, { props: { line: line('error') } }).body).toContain(
			'data-report-issue-button'
		);
		expect(render(ProcessLine, { props: { line: line('info') } }).body).not.toContain(
			'data-report-issue-button'
		);
	});
});
