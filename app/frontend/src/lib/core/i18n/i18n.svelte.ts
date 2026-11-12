// The app's words in the person's language. English ships in the bundle; other languages load when picked, and any missing line falls back to English.

export type Locale =
	| 'en' | 'es' | 'pt-br' | 'pt-pt' | 'fr' | 'de' | 'it' | 'nl' | 'sv' | 'nb' | 'da' | 'fi' | 'ru' | 'uk' | 'pl' | 'cs' | 'hu' | 'ro' | 'el'
	| 'sk' | 'bg' | 'sr' | 'hr' | 'ca' | 'lt' | 'lv' | 'et' | 'is'
	| 'zh-cn' | 'zh-tw' | 'ja' | 'ko' | 'hi' | 'bn' | 'ta' | 'id' | 'ms' | 'fil' | 'vi' | 'th' | 'tr' | 'he' | 'fa' | 'ar';
export type LocaleChoice = Locale | 'auto';

/** Each language named in itself, in the same order as holyarrs.com. */
export const LOCALES: { code: Locale; name: string }[] = [
	{ code: 'en', name: 'English' },
	{ code: 'es', name: 'Español' },
	{ code: 'pt-br', name: 'Português (Brasil)' },
	{ code: 'pt-pt', name: 'Português (Portugal)' },
	{ code: 'fr', name: 'Français' },
	{ code: 'de', name: 'Deutsch' },
	{ code: 'it', name: 'Italiano' },
	{ code: 'nl', name: 'Nederlands' },
	{ code: 'sv', name: 'Svenska' },
	{ code: 'nb', name: 'Norsk bokmål' },
	{ code: 'da', name: 'Dansk' },
	{ code: 'fi', name: 'Suomi' },
	{ code: 'ru', name: 'Русский' },
	{ code: 'uk', name: 'Українська' },
	{ code: 'pl', name: 'Polski' },
	{ code: 'cs', name: 'Čeština' },
	{ code: 'hu', name: 'Magyar' },
	{ code: 'ro', name: 'Română' },
	{ code: 'el', name: 'Ελληνικά' },
	{ code: 'sk', name: 'Slovenčina' },
	{ code: 'bg', name: 'Български' },
	{ code: 'sr', name: 'Српски' },
	{ code: 'hr', name: 'Hrvatski' },
	{ code: 'ca', name: 'Català' },
	{ code: 'lt', name: 'Lietuvių' },
	{ code: 'lv', name: 'Latviešu' },
	{ code: 'et', name: 'Eesti' },
	{ code: 'is', name: 'Íslenska' },
	{ code: 'zh-cn', name: '简体中文' },
	{ code: 'zh-tw', name: '繁體中文' },
	{ code: 'ja', name: '日本語' },
	{ code: 'ko', name: '한국어' },
	{ code: 'hi', name: 'हिन्दी' },
	{ code: 'bn', name: 'বাংলা' },
	{ code: 'ta', name: 'தமிழ்' },
	{ code: 'id', name: 'Bahasa Indonesia' },
	{ code: 'ms', name: 'Bahasa Melayu' },
	{ code: 'fil', name: 'Filipino' },
	{ code: 'vi', name: 'Tiếng Việt' },
	{ code: 'th', name: 'ไทย' },
	{ code: 'tr', name: 'Türkçe' },
	{ code: 'he', name: 'עברית' },
	{ code: 'fa', name: 'فارسی' },
	{ code: 'ar', name: 'العربية' }
];

const KEY = 'holyarrs-lang';
const RTL = new Set<Locale>(['ar', 'he', 'fa']);
type Words = Record<string, string>;
// One file per module per language (messages/<lang>/<module>.json), so the words sit beside each other but never collide.
const en: Words = merge(import.meta.glob<{ default: Words }>('./messages/en/*.json', { eager: true }));
// Other languages ship as one pack each (scripts/pack-lang.mjs): only the lines, in sorted English key order.
const packs = import.meta.glob<string>(['./packs/*.json', '!./packs/fit.json'], { query: '?url', import: 'default', eager: true });
const fit = (Object.values(import.meta.glob<{ fit: string }>('./packs/fit.json', { eager: true, import: 'default' }))[0] ?? { fit: '' }).fit;
const ORDER = Object.keys(en).sort();

export const lang = $state<{ value: Locale; choice: LocaleChoice; words: Record<string, string> }>({
	value: 'en',
	choice: 'auto',
	words: en
});

/** The line for `key` in the current language, with {name} filled from `vars`. */
export function t(key: string, vars?: Record<string, string | number>): string {
	const line = lang.words[key] ?? en[key] ?? key;
	return vars ? line.replace(/\{(\w+)\}/g, (m, k) => (k in vars ? String(vars[k]) : m)) : line;
}

/** The BCP-47 tag for dates and numbers in the app's language. English keeps the browser's own English (en-GB stays en-GB), or `en` when given. */
export function appLocale(en?: string): string | undefined {
	if (lang.value === 'en') {
		const b = typeof navigator === 'undefined' ? '' : navigator.language ?? '';
		return en ?? (b.toLowerCase().startsWith('en') ? b : 'en');
	}
	return bcp47(lang.value);
}

function bcp47(l: Locale): string {
	const [a, b] = l.split('-');
	return b ? `${a}-${b.toUpperCase()}` : l;
}

/** Picks a language; 'auto' follows the browser. Saved in this browser so the first paint is right. */
export async function setLang(choice: LocaleChoice): Promise<void> {
	lang.choice = choice;
	try {
		if (choice === 'auto') localStorage.removeItem(KEY);
		else localStorage.setItem(KEY, choice);
	} catch {
		// Private windows may refuse storage; the choice then lasts this visit.
	}
	await use(choice === 'auto' ? browserLocale() : choice);
}

/** Reads the saved or browser language at start. */
export function startLang(): Promise<void> {
	return setLang(storedChoice());
}

/** The closest supported language to what the browser asks for. */
export function browserLocale(asked: readonly string[] = typeof navigator === 'undefined' ? [] : navigator.languages ?? [navigator.language]): Locale {
	for (const a of asked) {
		const l = a.toLowerCase();
		const exact = LOCALES.find((x) => x.code === l);
		if (exact) return exact.code;
		const [b, region = ''] = l.split('-');
		if (b === 'pt') return ['br', ''].includes(region) ? 'pt-br' : 'pt-pt';
		if (b === 'zh') return /^(tw|hk|mo|hant)/.test(l.slice(3)) ? 'zh-tw' : 'zh-cn';
		if (b === 'no' || b === 'nn') return 'nb';
		if (b === 'iw') return 'he';
		if (b === 'in') return 'id';
		if (b === 'tl') return 'fil';
		if (b === 'sh') return 'sr';
		const base = LOCALES.find((x) => x.code === l.split('-')[0]);
		if (base) return base.code;
	}
	return 'en';
}

export function isLocaleChoice(v: unknown): v is LocaleChoice {
	return v === 'auto' || LOCALES.some((x) => x.code === v);
}

// The language asked for last; a slower earlier load never overwrites it.
let want: Locale = 'en';

async function use(l: Locale): Promise<void> {
	want = l;
	let words: Record<string, string> = en;
	if (l !== 'en') {
		try {
			const url = packs[`./packs/${l}.json`];
			if (!url) throw new Error('no pack');
			const res = await fetch(url);
			if (!res.ok) throw new Error('no pack');
			const p = (await res.json()) as { fit: string; values: (string | null)[] };
			if (p.fit !== fit || p.values.length !== ORDER.length) throw new Error('stale pack');
			if (want !== l) return;
			const got: Words = {};
			ORDER.forEach((k, i) => {
				const v = p.values[i];
				if (typeof v === 'string') got[k] = v;
			});
			words = { ...en, ...got };
		} catch {
			l = 'en';
		}
	}
	lang.value = l;
	lang.words = words;
	if (typeof document !== 'undefined') {
		document.documentElement.lang = bcp47(l);
		document.documentElement.dir = RTL.has(l) ? 'rtl' : 'ltr';
	}
}

function storedChoice(): LocaleChoice {
	try {
		const v = localStorage.getItem(KEY);
		return isLocaleChoice(v) ? v : 'auto';
	} catch {
		return 'auto';
	}
}

function merge(files: Record<string, { default: Words }>): Words {
	return Object.assign({}, ...Object.values(files).map((f) => f.default));
}
