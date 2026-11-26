// Language packs: one card in the plugin list; its sheet lists every language, English built in, installed packs first.
import { LOCALES } from '#lib/core/i18n/i18n.svelte.ts';
import type { LanguageRow } from '#lib/core/i18n/language-packs.svelte.ts';

const rank = (r: LanguageRow) => (r.state === 'built_in' ? 0 : r.state === 'installed' ? 1 : 2);
const at = (code: string) => LOCALES.findIndex((l) => l.code === code);

export function sortLanguageRows(rows: LanguageRow[]): LanguageRow[] {
	return [...rows].sort((a, b) => rank(a) - rank(b) || at(a.code) - at(b.code));
}

export function ownName(code: string): string {
	return LOCALES.find((l) => l.code === code)?.name ?? code;
}

export function englishName(code: string): string {
	try {
		const [a, b] = code.split('-');
		return new Intl.DisplayNames(['en'], { type: 'language' }).of(b ? `${a}-${b.toUpperCase()}` : a) ?? code;
	} catch {
		return code;
	}
}

export { installedCodes } from '#lib/core/i18n/language-packs.svelte.ts';

/** Whether a language matches the search text: its own name, English name or code. */
export function matchLanguage(code: string, term: string): boolean {
	const q = term.trim().toLocaleLowerCase();
	if (!q) return true;
	return [ownName(code), englishName(code), code].some((s) => s.toLocaleLowerCase().includes(q));
}

/** The Install or Remove button in a language's card, to take focus back after the card changes. */
export function languageAction(root: ParentNode, code: string): HTMLElement | null {
	return root.querySelector<HTMLElement>(`[data-testid="language-row"][data-code="${code.replace(/[^a-z0-9-]/gi, "")}"] button`);
}

/** The Language packs card: how many packs there are and how many are installed, or null when the search or filters hide it. */
/** Language Packs' own version as a plugin. The signed list's version is the app build the packs fit (for example 1.3.0), so it is never shown as this one. */
export const LANGUAGE_PACKS_VERSION = '1.0.0';

/** Language Packs' logo (same as the plugins repo's icon.svg), as a picture its window and tile can show and tint. */
export const LANGUAGE_PACKS_ICON = 'data:image/svg+xml,' + encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 128 128"><rect width="128" height="128" rx="28" fill="#1f2430"/><g fill="none" stroke="#fff" stroke-width="7" stroke-linecap="round" stroke-linejoin="round" transform="translate(16 16) scale(4)"><path d="M2.5 19l4-11 4 11M4 15h5M13.5 6.5h8M17.5 4v2.5M14.5 9.5c1.5 4 4 6.8 7 8.5M20.5 9.5c-1.5 4-4 6.8-7 8.5" stroke-width="1.75"/></g></svg>');

export function languageCard(installed: ReadonlySet<string> | null, term: string, tab: string, show: string, title = ''): { n: number; installed: number } | null {
	const codes = LOCALES.map((l) => l.code).filter((c) => c !== 'en');
	const have = codes.filter((c) => installed?.has(c)).length;
	const q = term.trim().toLocaleLowerCase();
	if (tab !== 'all' || show === 'updates' || (show === 'installed' && !have)) return null;
	if (q && !title.toLocaleLowerCase().includes(q) && !codes.some((c) => matchLanguage(c, q))) return null;
	return { n: codes.length, installed: have };
}
