import { afterEach, describe, expect, it, vi } from 'vitest';

import { get } from 'svelte/store';
import { locale } from 'svelte-i18n';

import { initI18n, setLanguage } from './index';

// These cover the two entry points the console starts and switches language
// through. Both write the document language, which screen readers and the
// browser's own text handling read, so the document element is stood in for.
afterEach(() => {
	vi.unstubAllGlobals();
	locale.set('en');
});

function stubDocument() {
	const element = { lang: '' };
	vi.stubGlobal('document', { documentElement: element });
	return element;
}

describe('initI18n', () => {
	it('starts rendering in the language the daemon handed the shell', async () => {
		const element = stubDocument();
		await initI18n('de');

		expect(get(locale)).toBe('de');
		expect(element.lang).toBe('de');
	});

	it('falls back to english for a language it does not ship', async () => {
		const element = stubDocument();
		await initI18n('sw');

		expect(get(locale)).toBe('en');
		expect(element.lang).toBe('en');
	});

	it('starts in english when no language is named', async () => {
		const element = stubDocument();
		await initI18n(undefined);

		expect(get(locale)).toBe('en');
		expect(element.lang).toBe('en');
	});

	it('starts in english when the language is the automatic choice', async () => {
		stubDocument();
		await initI18n(null);
		expect(get(locale)).toBe('en');
	});

	it('reads a regional tag as the base language it ships', async () => {
		const element = stubDocument();
		await initI18n('de-AT');

		expect(get(locale)).toBe('de');
		expect(element.lang).toBe('de');
	});

	it('renders a message once the catalogs are loaded', async () => {
		stubDocument();
		await initI18n('en');

		// A message the console draws on every page, read through the store the
		// components bind to.
		const { t } = await import('svelte-i18n');
		expect(get(t)('ui.common.requestFailed', { values: { status: 500 } })).toContain('500');
	});
});

describe('setLanguage', () => {
	it('switches the console over without a reload', async () => {
		const element = stubDocument();
		await initI18n('en');
		expect(element.lang).toBe('en');

		await setLanguage('fr');

		expect(get(locale)).toBe('fr');
		expect(element.lang).toBe('fr');
	});

	it('falls back to english for a language it does not ship', async () => {
		const element = stubDocument();
		await initI18n('de');

		await setLanguage('sw');

		expect(get(locale)).toBe('en');
		expect(element.lang).toBe('en');
	});

	it('treats no choice as english', async () => {
		const element = stubDocument();
		await initI18n('de');

		await setLanguage(undefined);

		expect(get(locale)).toBe('en');
		expect(element.lang).toBe('en');
	});
});
