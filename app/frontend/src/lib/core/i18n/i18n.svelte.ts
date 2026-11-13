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

/** A path or other left-to-right text kept in its own order inside a right-to-left line (Arabic, Hebrew, Persian); unchanged elsewhere. */
export function ltr(s: string): string {
	// Left-to-right isolate (U+2066) ... pop directional isolate (U+2069).
	return RTL.has(lang.value) ? String.fromCharCode(0x2066) + s + String.fromCharCode(0x2069) : s;
}

/** The BCP-47 tag for dates and numbers in the app's language. English keeps the browser's own English (en-GB stays en-GB), or `en` when given. */
export function appLocale(en?: string): string | undefined {
	if (lang.value === 'en') {
		const b = typeof navigator === 'undefined' ? '' : navigator.language ?? '';
		return en ?? (b.toLowerCase().startsWith('en') ? b : 'en');
	}
	// Thai defaults to the Buddhist calendar (2026 shows as 2569); air dates and release years must match the rest of the app.
	if (lang.value === 'th') return 'th-TH-u-ca-gregory';
	return bcp47(lang.value);
}

function bcp47(l: Locale): string {
	const [a, b] = l.split('-');
	return b ? `${a}-${b.toUpperCase()}` : l;
}

// The choice picked last in this visit; null until the app starts.
let picked: LocaleChoice | null = null;

/** The language the app is shown in (or is loading): the server sends titles and synopses in it. */
export function appLanguage(): Locale {
	const c = picked ?? storedChoice();
	return c === 'auto' ? browserLocale() : c;
}

/** Picks a language; 'auto' follows the browser. Saved in this browser so the first paint is right. */
export async function setLang(choice: LocaleChoice): Promise<void> {
	picked = choice;
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
		const got = await readPack(packs[`./packs/${l}.json`]);
		if (want !== l || got === 'reload') return;
		if (got) words = got;
		else l = 'en';
	}
	lang.value = l;
	lang.words = words;
	if (typeof document !== 'undefined') {
		document.documentElement.lang = bcp47(l);
		document.documentElement.dir = RTL.has(l) ? 'rtl' : 'ltr';
	}
}

const RELOADED = 'holyarrs-lang-reloaded';

/** One language's words from its pack (lines it lacks in English); null when the pack can't be used, 'reload' when the page reloads to get a newer build's packs. */
export async function readPack(url: string | undefined): Promise<Words | null | 'reload'> {
	try {
		if (!url) return null;
		const res = await fetch(url);
		// A tab from before an update asks for the old build's pack, which is gone: one reload brings the new page and its packs.
		if (res.status === 404 && reloadOnce()) return 'reload';
		if (!res.ok) return null;
		const p = (await res.json()) as { fit: string; values: (string | null)[] };
		if (p.fit !== fit || p.values.length !== ORDER.length) return null;
		try {
			sessionStorage.removeItem(RELOADED);
		} catch {
			// No storage: nothing was kept.
		}
		const got: Words = {};
		ORDER.forEach((k, i) => {
			const v = p.values[i];
			if (typeof v === 'string') got[k] = v;
		});
		return { ...en, ...got };
	} catch {
		return null;
	}
}

// Reloads the page unless this tab already did for a missing pack, so a pack that is really gone shows English instead of looping.
function reloadOnce(): boolean {
	try {
		if (sessionStorage.getItem(RELOADED)) return false;
		sessionStorage.setItem(RELOADED, '1');
		location.reload();
		return true;
	} catch {
		return false;
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
