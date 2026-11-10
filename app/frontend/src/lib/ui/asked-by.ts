
// Wording and helpers for the "who asked for this title" labels.

import { t, appLocale } from '#lib/core/i18n/i18n.svelte.ts';
export type Asker = { id: number; name: string; asked_at: string; me: boolean };
export type AskedByAnswer = { titles: Record<string, { askers: Asker[] }> };

// All label text lives here so wording changes in one place.
export const WORDS = {
	get you() {
		return t('ui.askedBy.you');
	},
	one: (a: string) => t('ui.askedBy.one', { name: a }),
	two: (a: string, b: string) => t('ui.askedBy.two', { a, b }),
	many: (a: string, others: number) => (others === 1 ? t('ui.askedBy.manyOne', { name: a }) : t('ui.askedBy.manyOther', { name: a, count: others })),
	youAnd: (others: number) => (others === 1 ? t('ui.askedBy.youAndOne') : t('ui.askedBy.youAndOther', { count: others })),
	tip: (who: string, date: string) => t('ui.askedBy.tip', { who, date }),
	youTip: (date: string) => t('ui.askedBy.youTip', { date }),
	dateSep: ' · ',
	get listTitle() {
		return t('ui.askedBy.listTitle');
	}
} as const;

// Falls back to the full name when it is blank so a label never shows empty.
export function firstName(name: string): string {
	return (name || '').trim().split(/\s+/)[0] || name;
}

// Bad dates give an empty string so the caller can skip the date part.
export function shortDate(iso: string): string {
	const d = new Date(iso);
	if (Number.isNaN(d.getTime())) return '';
	return d.toLocaleDateString(appLocale('en-GB'), { day: 'numeric', month: 'short' });
}

// Puts the viewer first and counts everyone else as "others".
export function askedByLine(askers: Asker[]): string {
	if (!askers.length) return '';
	const others = askers.filter((a) => !a.me);
	const me = askers.some((a) => a.me);
	if (me && !others.length) return WORDS.you;
	if (me) return WORDS.youAnd(others.length);
	const n = others.map((a) => firstName(a.name));
	if (n.length === 1) return WORDS.one(n[0]);
	if (n.length === 2) return WORDS.two(n[0], n[1]);
	return WORDS.many(n[0], n.length - 1);
}

// Tooltip names the first other person, since the viewer already knows their own ask.
export function askedByTip(askers: Asker[]): string {
	if (!askers.length) return '';
	const first = askers[0];
	const date = shortDate(first.asked_at);
	if (first.me && askers.length === 1) return WORDS.youTip(date);
	const other = askers.find((a) => !a.me) ?? first;
	const who = firstName(other.name);
	const when = shortDate(other.asked_at);
	return WORDS.tip(who, when);
}

export function askedByDated(askers: Asker[]): string {
	const line = askedByLine(askers);
	if (!line) return '';
	const date = shortDate(askers[0].asked_at);
	return date ? line + WORDS.dateSep + date : line;
}

export function initial(name: string): string {
	return (firstName(name)[0] || '?').toUpperCase();
}

// The server caps one request, so ids are deduped and cut to 200.
export async function loadAskedBy(get: <T>(path: string) => Promise<T>, ids: number[]): Promise<AskedByAnswer> {
	const uniq = [...new Set(ids.filter((n) => n > 0))].slice(0, 200);
	if (!uniq.length) return { titles: {} };
	return get<AskedByAnswer>(`/asked-by?title_ids=${uniq.join(',')}`);
}
