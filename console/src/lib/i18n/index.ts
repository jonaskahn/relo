import { get, writable } from 'svelte/store';
import { addMessages, getLocaleFromNavigator, init, locale, waitLocale } from 'svelte-i18n';

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
import ptBr from './locales/pt-BR.json';
import ro from './locales/ro.json';
import ru from './locales/ru.json';
import sv from './locales/sv.json';
import th from './locales/th.json';
import tr from './locales/tr.json';
import uk from './locales/uk.json';
import vi from './locales/vi.json';
import ar from './locales/ar.json';
import zhHans from './locales/zh-Hans.json';
import zhHant from './locales/zh-Hant.json';

/** The languages the console ships, in the order the picker lists them. */
export const locales = [
	'en',
	'de',
	'zh-Hans',
	'ar',
	'cs',
	'es',
	'fr',
	'hi',
	'id',
	'it',
	'ja',
	'ko',
	'nl',
	'pl',
	'pt-BR',
	'ro',
	'ru',
	'sv',
	'th',
	'tr',
	'uk',
	'vi',
	'zh-Hant'
] as const;

/** A shipped catalog tag. */
export type Locale = (typeof locales)[number];

/** Reports whether a tag the shell, a stored choice, or a query string carries is one the
 *  console ships a catalog for. */
export function isLocale(value: string): value is Locale {
	return locales.some((candidate) => candidate === value);
}

// The bundled catalog behind each language, keyed by its tag.
const catalogs = {
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

/** The choice that follows the system locale, which the daemon resolves before it hands the page
 *  a language. */
export const auto = 'auto';

/** The console language picker order, including "auto". */
export const languageChoices = [auto, ...locales] as const;

/** One entry of the language picker. */
export type LanguageChoice = (typeof languageChoices)[number];

/** Reports whether a value a picker handed back, or a saved choice holds, is one the console
 *  offers, so an older or stray value is not applied as a language it has no catalog for. */
export function isLanguageChoice(value: string): value is LanguageChoice {
	return languageChoices.some((choice) => choice === value);
}

/** The choice stored by the daemon, including "auto".
 *  The rendered locale can differ when that choice follows the system. */
export const savedLanguage = writable<string>(auto);

/** Names each language the way its own speakers write it, so the picker stays readable whatever
 *  the console currently speaks. */
export const languageNames: Record<Locale, string> = {
	en: 'English',
	de: 'Deutsch',
	'zh-Hans': '简体中文',
	ar: 'العربية',
	cs: 'Čeština',
	es: 'Español',
	fr: 'Français',
	hi: 'हिन्दी',
	id: 'Bahasa Indonesia',
	it: 'Italiano',
	ja: '日本語',
	ko: '한국어',
	nl: 'Nederlands',
	pl: 'Polski',
	'pt-BR': 'Português (Brasil)',
	ro: 'Română',
	ru: 'Русский',
	sv: 'Svenska',
	th: 'ไทย',
	tr: 'Türkçe',
	uk: 'Українська',
	vi: 'Tiếng Việt',
	'zh-Hant': '繁體中文'
};

/** Gives each locale a regional flag emoji for the picker. */
export const languageFlags: Record<Locale, string> = {
	en: '🇺🇸',
	de: '🇩🇪',
	'zh-Hans': '🇨🇳',
	ar: '🇸🇦',
	cs: '🇨🇿',
	es: '🇪🇸',
	fr: '🇫🇷',
	hi: '🇮🇳',
	id: '🇮🇩',
	it: '🇮🇹',
	ja: '🇯🇵',
	ko: '🇰🇷',
	nl: '🇳🇱',
	pl: '🇵🇱',
	'pt-BR': '🇧🇷',
	ro: '🇷🇴',
	ru: '🇷🇺',
	sv: '🇸🇪',
	th: '🇹🇭',
	tr: '🇹🇷',
	uk: '🇺🇦',
	vi: '🇻🇳',
	'zh-Hant': '🇹🇼'
};

const languageAutoFlag = '🌐';

/** Formats a native select row as flag plus name. */
export function languageOptionLabel(code: LanguageChoice, autoLabel: string): string {
	if (code === auto) {
		return `${languageAutoFlag} ${autoLabel}`;
	}
	return `${languageFlags[code]} ${languageNames[code]}`;
}

/** Maps any tag onto the language the console renders in.
 *  A regional tag such as de-AT or pt-PT belongs to its base language, and a tag the console
 *  ships no catalog for falls back to English.
 *  Chinese stays script-aware: an explicit Traditional script or a Traditional region (TW, HK,
 *  MO) reads as Traditional, everything else Simplified. */
export function resolve(language: string | null | undefined): Locale {
	if (language && isLocale(language)) {
		return language;
	}
	if (!language) {
		return 'en';
	}
	if (language.includes('Hant')) {
		return 'zh-Hant';
	}
	const [base, region] = language.split('-');
	if (base === 'zh') {
		return ['TW', 'HK', 'MO'].includes((region ?? '').toUpperCase()) ? 'zh-Hant' : 'zh-Hans';
	}
	return (
		locales.find((candidate) => candidate === base || candidate.split('-')[0] === base) ?? 'en'
	);
}

/** Loads the bundled catalogs and starts rendering in the language the daemon handed the shell.
 *  The layout awaits it, so no page paints in the wrong language first. */
export async function initI18n(language: string | null | undefined): Promise<void> {
	for (const tag of locales) {
		addMessages(tag, catalogs[tag]);
	}
	init({ fallbackLocale: 'en', initialLocale: resolve(language ?? getLocaleFromNavigator()) });
	await waitLocale();
	applyDocumentLanguage();
}

/** Switches the console over without a reload, which is what a saved choice does. */
export async function setLanguage(language: string | null | undefined): Promise<void> {
	locale.set(resolve(language));
	await waitLocale();
	applyDocumentLanguage();
}

function applyDocumentLanguage(): void {
	const tag = get(locale) ?? 'en';
	document.documentElement.lang = tag;
}
