import { describe, expect, it } from 'vitest';

import { locale } from 'svelte-i18n';

import { acceptLanguage } from '../api';
import {
	auto,
	isLanguageChoice,
	isLocale,
	languageFlags,
	languageNames,
	languageOptionLabel,
	locales,
	resolve
} from './index';

describe('resolve', () => {
	it('keeps a language the console ships', () => {
		for (const tag of locales) {
			expect(resolve(tag)).toBe(tag);
		}
		expect(resolve('de')).toBe('de');
		expect(resolve('zh-Hans')).toBe('zh-Hans');
		expect(resolve('zh-Hant')).toBe('zh-Hant');
		expect(resolve('pt-BR')).toBe('pt-BR');
	});

	it('reads a regional tag as its base language', () => {
		expect(resolve('de-AT')).toBe('de');
		expect(resolve('en-GB')).toBe('en');
		expect(resolve('fr-CA')).toBe('fr');
		expect(resolve('es-MX')).toBe('es');
		expect(resolve('pt-PT')).toBe('pt-BR');
		expect(resolve('ja-JP')).toBe('ja');
	});

	it('reads a chinese tag by script and region', () => {
		// The console only ever sees a tag the daemon resolved, but a page
		// opened outside it should still land somewhere sensible.
		expect(resolve('zh')).toBe('zh-Hans');
		expect(resolve('zh-CN')).toBe('zh-Hans');
		expect(resolve('zh-SG')).toBe('zh-Hans');
		expect(resolve('zh-Hant')).toBe('zh-Hant');
		expect(resolve('zh-Hant-TW')).toBe('zh-Hant');
		expect(resolve('zh-TW')).toBe('zh-Hant');
		expect(resolve('zh-HK')).toBe('zh-Hant');
		expect(resolve('zh-MO')).toBe('zh-Hant');
	});

	it('falls back to english for anything else', () => {
		expect(resolve('sw')).toBe('en');
		expect(resolve('auto')).toBe('en');
		expect(resolve(undefined)).toBe('en');
		expect(resolve('')).toBe('en');
	});
});

describe('languageFlags', () => {
	it('flags every language it ships', () => {
		for (const tag of locales) {
			expect(languageFlags[tag]).toBeTruthy();
		}
	});
});

describe('languageOptionLabel', () => {
	it('prefixes auto and each locale with a flag', () => {
		expect(languageOptionLabel(auto, 'System')).toMatch(/^🌐 System$/);
		expect(languageOptionLabel('en', 'System')).toBe(`${languageFlags.en} English`);
		expect(languageOptionLabel('de', 'System')).toBe(`${languageFlags.de} Deutsch`);
	});
});

describe('languageNames', () => {
	it('names every language it ships', () => {
		expect(locales).toHaveLength(23);
		for (const tag of locales) {
			expect(languageNames[tag]).toBeTruthy();
		}
	});

	it('names each language the way its own speakers write it', () => {
		expect(languageNames['ja']).toBe('日本語');
		expect(languageNames['pt-BR']).toBe('Português (Brasil)');
		expect(languageNames['zh-Hant']).toBe('繁體中文');
	});
});

describe('acceptLanguage', () => {
	it('asks the daemon for the language the console renders in', () => {
		locale.set('de');
		expect(acceptLanguage()).toBe('de');
		locale.set('zh-Hans');
		expect(acceptLanguage()).toBe('zh-Hans');
		locale.set('fr');
		expect(acceptLanguage()).toBe('fr');
		locale.set('en');
	});
});

describe('the guards over a shipped tag', () => {
	it('accepts a catalog the console ships and refuses one it does not', () => {
		expect(isLocale('pt-BR')).toBe(true);
		expect(isLocale('en-GB')).toBe(false);
		expect(isLocale('')).toBe(false);
	});

	it('accepts a picker choice, including the one that follows the system', () => {
		expect(isLanguageChoice(auto)).toBe(true);
		expect(isLanguageChoice('de')).toBe(true);
		expect(isLanguageChoice('eo')).toBe(false);
	});
});
