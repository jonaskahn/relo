import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

import { getMessageFormatter, locale } from 'svelte-i18n';

import { locales } from './index';
import cs from './locales/cs.json';
import de from './locales/de.json';
import en from './locales/en.json';
import es from './locales/es.json';
import fr from './locales/fr.json';
import hi from './locales/hi.json';
import id from './locales/id.json';
import itJson from './locales/it.json';
import ja from './locales/ja.json';
import ko from './locales/ko.json';
import nl from './locales/nl.json';
import pl from './locales/pl.json';
import ar from './locales/ar.json';
import ptBr from './locales/pt-BR.json';
import ro from './locales/ro.json';
import ru from './locales/ru.json';
import sv from './locales/sv.json';
import th from './locales/th.json';
import tr from './locales/tr.json';
import uk from './locales/uk.json';
import vi from './locales/vi.json';
import zhHans from './locales/zh-Hans.json';
import zhHant from './locales/zh-Hant.json';

const catalogs: Record<string, unknown> = {
	en,
	de,
	'zh-Hans': zhHans,
	ar,
	cs,
	es,
	fr,
	hi,
	id,
	it: itJson,
	ja,
	ko,
	nl,
	pl,
	'pt-BR': ptBr,
	ro,
	ru,
	sv,
	th,
	tr,
	uk,
	vi,
	'zh-Hant': zhHant
};

function keysOf(catalog: unknown, prefix = ''): string[] {
	if (typeof catalog !== 'object' || catalog === null) {
		return [prefix];
	}
	return Object.entries(catalog).flatMap(([key, value]) =>
		keysOf(value, prefix ? prefix + '.' + key : key)
	);
}

// The tags the Go catalogs ship, so the console cannot drift from the list
// the daemon answers with.
function daemonCatalogs(): string[] {
	return readdirSync('../daemon/internal/i18n/catalogs')
		.filter((name) => name.startsWith('active.') && name.endsWith('.toml'))
		.map((name) => name.slice('active.'.length, -'.toml'.length))
		.sort();
}

function valueOf(catalog: unknown, key: string): string {
	const found = key.split('.').reduce<unknown>((node, part) => {
		return typeof node === 'object' && node !== null ? Reflect.get(node, part) : undefined;
	}, catalog);
	return typeof found === 'string' ? found : '';
}

describe('console catalogs', () => {
	it('ships exactly the languages the daemon ships', () => {
		expect([...locales].sort()).toEqual(daemonCatalogs());
	});

	it('keeps a locale store and a catalog per language', () => {
		expect(Object.keys(catalogs).sort()).toEqual(daemonCatalogs());
		expect(locale).toBeDefined();
	});

	it('declares the same messages in every language', () => {
		const reference = keysOf(en).sort();
		expect(reference.length).toBeGreaterThan(0);
		for (const [tag, catalog] of Object.entries(catalogs)) {
			expect(keysOf(catalog).sort(), tag).toEqual(reference);
		}
	});

	it('leaves no message empty', () => {
		for (const [tag, catalog] of Object.entries(catalogs)) {
			for (const key of keysOf(catalog)) {
				const value = valueOf(catalog, key);
				expect(typeof value, tag + ' ' + key).toBe('string');
				expect((value as string).trim(), tag + ' ' + key).not.toBe('');
			}
		}
	});

	// A plural message with broken ICU syntax would only surface in the UI,
	// so every count a surface can show renders here first.
	it('renders every plural message for zero, one, and many', () => {
		let plurals = 0;
		for (const [tag, catalog] of Object.entries(catalogs)) {
			for (const key of keysOf(catalog)) {
				const message = valueOf(catalog, key);
				if (!message.includes(', plural,')) continue;
				plurals++;
				const formatter = getMessageFormatter(message, tag);
				for (const count of [0, 1, 2]) {
					const rendered = String(formatter.format({ count }));
					expect(rendered, tag + ' ' + key + ' count=' + count).not.toContain('{');
					expect(rendered.trim(), tag + ' ' + key + ' count=' + count).not.toBe('');
				}
			}
		}
		expect(plurals).toBeGreaterThan(0);
	});

	// Every message carries the same placeholders as the English it mirrors.
	// A renamed or dropped variable renders as literal text in the interface,
	// so the drift is checked here rather than noticed in production.
	it('keeps every placeholder the English message declares', () => {
		const pattern = /\{[a-zA-Z][a-zA-Z0-9_]*\}|#[a-zA-Z]/g;
		const reference = new Map(keysOf(en).map((key) => [key, valueOf(en, key)]));
		for (const [tag, catalog] of Object.entries(catalogs)) {
			for (const key of keysOf(catalog)) {
				const want = (reference.get(key) as string).match(pattern)?.sort() ?? [];
				const got = (valueOf(catalog, key) as string).match(pattern)?.sort() ?? [];
				expect(got, tag + ' ' + key).toEqual(want);
			}
		}
	});

	// Russian, Ukrainian, Polish, Czech, and Romanian need more plural
	// categories than one/other. Declaring only those two renders "3 аккаунтов"
	// where the language requires "3 аккаунта", so the categories each language
	// actually uses are asserted against the CLDR set.
	it('declares every plural category its language uses', () => {
		const required: Record<string, string[]> = {
			fr: ['one', 'many', 'other'],
			ru: ['one', 'few', 'many', 'other'],
			uk: ['one', 'few', 'many', 'other'],
			pl: ['one', 'few', 'many', 'other'],
			cs: ['one', 'few', 'many', 'other'],
			ro: ['one', 'few', 'other'],
			ja: ['other'],
			ko: ['other'],
			th: ['other'],
			hi: ['other'],
			'zh-Hans': ['other'],
			'zh-Hant': ['other']
		};
		for (const [tag, catalog] of Object.entries(catalogs)) {
			const need = required[tag] ?? ['one', 'other'];
			for (const key of keysOf(catalog)) {
				const message = valueOf(catalog, key) as string;
				if (!message.includes(', plural,')) continue;
				const declared = message.match(/\b(zero|one|two|few|many|other)\s*\{/g) ?? [];
				const have = new Set(declared.map((c) => c.replace(/\s*\{$/, '')));
				for (const category of need) {
					expect(have.has(category), tag + ' ' + key + ' needs plural category ' + category).toBe(
						true
					);
				}
			}
		}
	});

	// Three dots are not an ellipsis, and a translated catalog that drifts to
	// them looks like it came from a different product than the one beside it.
	it('uses the typographic ellipsis, never three dots', () => {
		for (const [tag, catalog] of Object.entries(catalogs)) {
			for (const key of keysOf(catalog)) {
				expect(valueOf(catalog, key) as string, tag + ' ' + key).not.toContain('...');
			}
		}
	});

	// German software addressed as "Sie" in a handful of strings while the rest
	// of the same file says "du" is the single most common register defect, so
	// the formal address is banned outright.
	it('keeps German informal and free of formal address', () => {
		const german: unknown = catalogs.de;
		for (const key of keysOf(german)) {
			const value = valueOf(german, key);
			expect(value, 'de ' + key).not.toMatch(/\b(Sie|Ihnen|Ihr|Ihre|Ihren|Ihrem)\b/);
		}
	});
});

// Every message one console namespace asks for, as the dotted key and the
// file that names it.
function namespaceKeysUsed(namespace: string): Map<string, string> {
	const used = new Map<string, string>();
	const pattern = new RegExp(namespace.replaceAll('.', '\\.') + '\\.([A-Za-z0-9_.]+)', 'g');
	for (const path of sourceFiles('src')) {
		const text = readFileSync(path, 'utf8');
		for (const match of text.matchAll(pattern)) {
			const key = namespace + '.' + match[1];
			if (!used.has(key)) used.set(key, path);
		}
	}
	return used;
}

function sourceFiles(directory: string): string[] {
	const out: string[] = [];
	for (const name of readdirSync(directory)) {
		const path = join(directory, name);
		if (statSync(path).isDirectory()) {
			out.push(...sourceFiles(path));
			continue;
		}
		if (name.endsWith('.svelte') || name.endsWith('.ts')) out.push(path);
	}
	return out;
}

describe('page messages', () => {
	// A key composed at runtime, such as a section suffix, is present as a
	// prefix of the keys it builds, so a prefix counts as covered.
	function expectCovered(used: Map<string, string>) {
		const keys = keysOf(en);
		for (const [key, file] of used) {
			const covered = keys.includes(key) || keys.some((candidate) => candidate.startsWith(key));
			expect(covered, key + ' used by ' + file).toBe(true);
		}
	}

	it('names a message for every providers page key the console uses', () => {
		expectCovered(namespaceKeysUsed('ui.pages.providersPage'));
	});

	it('names a message for every settings page key the console uses', () => {
		expectCovered(namespaceKeysUsed('ui.settingsPage'));
	});
});
