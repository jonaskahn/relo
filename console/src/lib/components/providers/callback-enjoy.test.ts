import { render } from 'svelte/server';
import { addMessages, init } from 'svelte-i18n';
import { beforeAll, describe, expect, it } from 'vitest';

import en from '$lib/i18n/locales/en.json';

import CallbackEnjoy from './callback-enjoy.svelte';

// The panel the callback page ends on, once the wind has taken the result away.
// It has to be honest about the outcome in both directions: a tab that closes
// itself has no time to explain, so this is the last thing the operator reads.

beforeAll(() => {
	addMessages('en', en);
	init({ fallbackLocale: 'en', initialLocale: 'en' });
});

function markup(props: { ok: boolean; account?: string }): string {
	return render(CallbackEnjoy, { props }).body;
}

describe('the panel a settled login ends on', () => {
	it('says what worked, and names the account', () => {
		const body = markup({ ok: true, account: 'me@example.com' });
		expect(body).toContain('Enjoy');
		expect(body).toContain('me@example.com');
		// The retry line is the opposite outcome, so it must not appear here.
		expect(body).not.toContain('That did not work');
	});

	it('says what to do when it did not work', () => {
		const body = markup({ ok: false, account: '' });
		expect(body).toContain('That did not work');
		expect(body).toContain('again');
	});

	it('never promises an account it was not given', () => {
		// A login can fail after the page learned an account name, and one can
		// expire with no name at all. Neither may put a blank or a stale account
		// on a screen the operator has one second to read.
		expect(markup({ ok: true, account: '' })).not.toContain('undefined');
		expect(markup({ ok: false, account: 'stale@example.com' })).not.toContain('stale@example.com');
	});
});
