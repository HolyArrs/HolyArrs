// What an ask carries from the metadata answer (seasons, release dates, language), the audio languages an approver can ask for, and who asked.

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

/** When something happened ("Mar 3") in the viewer's own time zone, never later than now. */
export function whenDate(iso: string, now = new Date()): string {
	const t = new Date(iso);
	if (Number.isNaN(t.getTime())) return '';
	const d = t > now ? now : t;
	const opts: Intl.DateTimeFormatOptions = { month: 'short', day: 'numeric' };
	if (d.getFullYear() !== now.getFullYear()) opts.year = 'numeric';
	return d.toLocaleDateString('en-US', opts);
}

/** When something comes next, in the viewer's time zone: "tonight at 11:19 PM", "today at 2:00 PM", "tomorrow at 5:19 AM", else "Nov 6". A time already gone reads "soon". */
export function nextWhen(iso: string, now = new Date()): string {
	const t = new Date(iso);
	if (Number.isNaN(t.getTime())) return '';
	if (t <= now) return 'soon';
	const at = t.toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit' });
	const midnight = new Date(now.getFullYear(), now.getMonth(), now.getDate() + 1);
	if (t < midnight) return `${t.getHours() >= 18 ? 'tonight' : 'today'} at ${at}`;
	if (t < new Date(now.getFullYear(), now.getMonth(), now.getDate() + 2)) return `tomorrow at ${at}`;
	const opts: Intl.DateTimeFormatOptions = { month: 'short', day: 'numeric' };
	if (t.getFullYear() !== now.getFullYear()) opts.year = 'numeric';
	return t.toLocaleDateString('en-US', opts);
}

/** "Mar 3", or with the year added outside this year. */
export function shortDate(iso: string, now = new Date()): string {
	const [y, m, d] = day(iso).split('-').map(Number);
	if (!y || !m || !d) return '';
	const t = new Date(Date.UTC(y, m - 1, d));
	const opts: Intl.DateTimeFormatOptions = { month: 'short', day: 'numeric', timeZone: 'UTC' };
	if (y !== now.getFullYear()) opts.year = 'numeric';
	return t.toLocaleDateString('en-US', opts);
}

/** The audio languages the approver can ask for, by base code. */
export const audioLanguages: { code: string; name: string }[] = [
	{ code: 'en', name: 'English' },
	{ code: 'es', name: 'Spanish' },
	{ code: 'fr', name: 'French' },
	{ code: 'de', name: 'German' },
	{ code: 'it', name: 'Italian' },
	{ code: 'pt', name: 'Portuguese' },
	{ code: 'nl', name: 'Dutch' },
	{ code: 'ja', name: 'Japanese' },
	{ code: 'ko', name: 'Korean' },
	{ code: 'zh', name: 'Chinese' },
	{ code: 'hi', name: 'Hindi' },
	{ code: 'ru', name: 'Russian' },
	{ code: 'pl', name: 'Polish' },
	{ code: 'sv', name: 'Swedish' },
	{ code: 'tr', name: 'Turkish' }
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
