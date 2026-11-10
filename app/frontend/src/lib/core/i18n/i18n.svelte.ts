// The app's words in the person's language. English ships in the bundle; other languages load when picked, and any missing line falls back to English.

export type Locale = 'en' | 'es' | 'pt-br' | 'fr' | 'de' | 'ru' | 'uk' | 'zh-cn' | 'ja' | 'ko' | 'hi' | 'ar' | 'it' | 'nl' | 'pl' | 'tr';
export type LocaleChoice = Locale | 'auto';

/** Each language named in itself, in the same order as holyarrs.com. */
export const LOCALES: { code: Locale; name: string }[] = [
	{ code: 'en', name: 'English' },
	{ code: 'es', name: 'Español' },
	{ code: 'pt-br', name: 'Português (Brasil)' },
	{ code: 'fr', name: 'Français' },
	{ code: 'de', name: 'Deutsch' },
	{ code: 'it', name: 'Italiano' },
	{ code: 'nl', name: 'Nederlands' },
	{ code: 'ru', name: 'Русский' },
	{ code: 'uk', name: 'Українська' },
	{ code: 'pl', name: 'Polski' },
	{ code: 'zh-cn', name: '简体中文' },
	{ code: 'ja', name: '日本語' },
	{ code: 'ko', name: '한국어' },
	{ code: 'hi', name: 'हिन्दी' },
	{ code: 'tr', name: 'Türkçe' },
	{ code: 'ar', name: 'العربية' }
];

const KEY = 'holyarrs-lang';
const RTL = new Set<Locale>(['ar']);
type Words = Record<string, string>;
// One file per module per language (messages/<lang>/<module>.json), so the words sit beside each other but never collide.
const en: Words = merge(import.meta.glob<{ default: Words }>('./messages/en/*.json', { eager: true }));
const lazy = import.meta.glob<{ default: Words }>(['./messages/*/*.json', '!./messages/en/*.json']);

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
	return l === 'pt-br' ? 'pt-BR' : l === 'zh-cn' ? 'zh-CN' : l;
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
		if (l.startsWith('pt')) return 'pt-br';
		if (l.startsWith('zh')) return 'zh-cn';
		const base = LOCALES.find((x) => x.code === l.split('-')[0]);
		if (base) return base.code;
	}
	return 'en';
}

export function isLocaleChoice(v: unknown): v is LocaleChoice {
	return v === 'auto' || LOCALES.some((x) => x.code === v);
}

async function use(l: Locale): Promise<void> {
	let words: Record<string, string> = en;
	if (l !== 'en') {
		try {
			const files = Object.entries(lazy).filter(([p]) => p.startsWith(`./messages/${l}/`));
			words = { ...en, ...merge(Object.fromEntries(await Promise.all(files.map(async ([p, load]) => [p, await load()])))) };
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
