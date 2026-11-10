// What an ask carries from the metadata answer (seasons, release dates, language), the audio languages an approver can ask for, and who asked.

import { lang, t, appLocale } from '#lib/core/i18n/i18n.svelte.ts';
import type { Ask } from '../core/types.ts';

/** A season as the metadata answer lists it. */
export interface SeasonInfo {
	number: number;
	episode_count: number;
	year?: number;
}

/** An episode as a season's metadata answer lists it. */
export interface EpisodeInfo {
	number: number;
	name?: string;
	air_day?: string;
}

/** What the metadata answer adds to an ask's summary. */
export interface AskInfo {
	seasons: SeasonInfo[];
	/** Movies: theaters, digital, physical. Shows: first_air, last_air. */
	dates: Record<string, string>;
	status?: string;
	originalLanguage?: string;
}

/** Picks the summary's parts out of a /metadata answer, tolerating missing fields. */
export function readInfo(raw: Record<string, unknown>): AskInfo {
	const seasons = Array.isArray(raw.seasons)
		? (raw.seasons as Record<string, unknown>[])
				.filter((s) => s && typeof s.number === 'number')
				.map((s) => ({ number: s.number as number, episode_count: typeof s.episode_count === 'number' ? s.episode_count : 0, year: typeof s.year === 'number' ? s.year : undefined }))
		: [];
	const d = raw.dates && typeof raw.dates === 'object' ? (raw.dates as Record<string, unknown>) : {};
	const dates = Object.fromEntries(Object.entries(d).filter((e): e is [string, string] => typeof e[1] === 'string' && !!e[1]));
	return {
		seasons,
		dates,
		status: typeof raw.status === 'string' ? raw.status : undefined,
		originalLanguage: typeof raw.original_language === 'string' && raw.original_language ? raw.original_language : undefined
	};
}

const day = (iso: string) => iso.slice(0, 10);

/** The language dates are written in: English keeps the US form. */
const dateLocale = () => appLocale('en-US');

/** When something happened ("Mar 3") in the viewer's own time zone, never later than now. */
export function whenDate(iso: string, now = new Date()): string {
	const at = new Date(iso);
	if (Number.isNaN(at.getTime())) return '';
	const d = at > now ? now : at;
	const opts: Intl.DateTimeFormatOptions = { month: 'short', day: 'numeric' };
	if (d.getFullYear() !== now.getFullYear()) opts.year = 'numeric';
	return d.toLocaleDateString(dateLocale(), opts);
}

/** When something comes next, in the viewer's time zone: "tonight at 11:19 PM", "today at 2:00 PM", "tomorrow at 5:19 AM", else "Nov 6". A time already gone reads "soon". */
export function nextWhen(iso: string, now = new Date()): string {
	const tm = new Date(iso);
	if (Number.isNaN(tm.getTime())) return '';
	if (tm <= now) return t('requests.when.soon');
	const at = tm.toLocaleTimeString(dateLocale(), { hour: 'numeric', minute: '2-digit' });
	const midnight = new Date(now.getFullYear(), now.getMonth(), now.getDate() + 1);
	if (tm < midnight) return t(tm.getHours() >= 18 ? 'requests.when.tonight' : 'requests.when.today', { at });
	if (tm < new Date(now.getFullYear(), now.getMonth(), now.getDate() + 2)) return t('requests.when.tomorrow', { at });
	const opts: Intl.DateTimeFormatOptions = { month: 'short', day: 'numeric' };
	if (tm.getFullYear() !== now.getFullYear()) opts.year = 'numeric';
	return tm.toLocaleDateString(dateLocale(), opts);
}

/** "Mar 3", or with the year added outside this year. */
export function shortDate(iso: string, now = new Date()): string {
	const [y, m, d] = day(iso).split('-').map(Number);
	if (!y || !m || !d) return '';
	const at = new Date(Date.UTC(y, m - 1, d));
	const opts: Intl.DateTimeFormatOptions = { month: 'short', day: 'numeric', timeZone: 'UTC' };
	if (y !== now.getFullYear()) opts.year = 'numeric';
	return at.toLocaleDateString(dateLocale(), opts);
}

/** The audio languages the approver can ask for, by base code. */
export const audioLanguages: { code: string; name: string }[] = [
	{ code: 'en', get name() { return t('requests.language.en'); } },
	{ code: 'es', get name() { return t('requests.language.es'); } },
	{ code: 'fr', get name() { return t('requests.language.fr'); } },
	{ code: 'de', get name() { return t('requests.language.de'); } },
	{ code: 'it', get name() { return t('requests.language.it'); } },
	{ code: 'pt', get name() { return t('requests.language.pt'); } },
	{ code: 'nl', get name() { return t('requests.language.nl'); } },
	{ code: 'ja', get name() { return t('requests.language.ja'); } },
	{ code: 'ko', get name() { return t('requests.language.ko'); } },
	{ code: 'zh', get name() { return t('requests.language.zh'); } },
	{ code: 'hi', get name() { return t('requests.language.hi'); } },
	{ code: 'ru', get name() { return t('requests.language.ru'); } },
	{ code: 'pl', get name() { return t('requests.language.pl'); } },
	{ code: 'sv', get name() { return t('requests.language.sv'); } },
	{ code: 'tr', get name() { return t('requests.language.tr'); } }
];

/** Names a base code ("ko" is Korean); unknown codes read as themselves. */
export function languageName(code: string | undefined): string {
	if (!code) return '';
	return audioLanguages.find((l) => l.code === code)?.name ?? code;
}

/** The people with asks, by name, for the Asked by filter. */
export function askers(asks: Pick<Ask, 'asked_by' | 'people'>[]): { id: number; name: string; n: number }[] {
	const by = new Map<number, { id: number; name: string; n: number }>();
	for (const a of asks) {
		const w = a.people[0] ?? a.asked_by;
		const g = by.get(w.id) ?? { id: w.id, name: w.name, n: 0 };
		g.n++;
		by.set(w.id, g);
	}
	return [...by.values()].sort((x, y) => x.name.localeCompare(y.name));
}

/** Whether someone is among the people who asked. */
export function askedByOne(a: Pick<Ask, 'asked_by' | 'people'>, id: number): boolean {
	return a.asked_by.id === id || a.people.some((p) => p.id === id);
}
