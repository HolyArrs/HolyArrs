// The language list: the most used first, the rest by name, and a search that matches
// a language's own name, its name in the page language, its English name or its code.
import { LOCALES, type Locale } from '#lib/core/i18n/i18n.svelte.ts';

export type Language = { code: Locale; name: string };

const TOP = ['en', 'es', 'pt-br', 'fr', 'de', 'it', 'ru', 'zh-cn', 'ja', 'ar'];

/**
 * installed: English and the installed packs, once known (null shows every language); choice stays listed
 * either way, so the current pick never disappears from the list.
 */
export function languageGroups(installed: ReadonlySet<string> | null = null, choice = 'en'): { top: Language[]; more: Language[] } {
	const list = installed ? LOCALES.filter((l) => l.code === 'en' || l.code === choice || installed.has(l.code)) : LOCALES;
	const top = list.filter((l) => TOP.includes(l.code));
	const more = list.filter((l) => !TOP.includes(l.code)).sort((a, b) => a.name.localeCompare(b.name));
	return { top, more };
}

const fold = (s: string) => s.normalize('NFD').replace(/\p{M}/gu, '').toLowerCase();

function nameIn(page: string, code: string): string {
	try {
		return new Intl.DisplayNames([page], { type: 'language' }).of(code) ?? '';
	} catch {
		return '';
	}
}

// Keeps the list order: Most used, then More languages.
export function matchLanguages(query: string, page: string, installed: ReadonlySet<string> | null = null, choice = 'en'): Language[] {
	const { top, more } = languageGroups(installed, choice);
	const all = [...top, ...more];
	const q = fold(query.trim());
	if (!q) return all;
	return all.filter((l) => [l.name, l.code, nameIn(page, l.code), nameIn('en', l.code)].some((s) => fold(s).includes(q)));
}

// The browser's own language name, shown beside Automatic.
export function languageName(code: string): string {
	return LOCALES.find((l) => l.code === code)?.name ?? code;
}
