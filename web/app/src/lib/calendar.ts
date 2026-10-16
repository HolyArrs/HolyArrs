// Calendar: the month grid and list for movie dates and episodes, and the
// feed link.
import type { CalendarEntry } from './types.ts';

export const kindWord: Record<CalendarEntry['kind'], string> = {
	cinema: 'In cinemas',
	digital: 'Digital release',
	physical: 'Disc release',
	episode: 'Episode',
	album: 'Album release'
};
export const kindShort: Record<CalendarEntry['kind'], string> = { cinema: 'Cinema', digital: 'Digital', physical: 'Disc', episode: 'Episode', album: 'Album' };

/** Which titles the calendar shows. */
export type TitlesShown = 'all' | 'movies' | 'shows' | 'music';

/** shows says whether an entry is shown for the movies/shows/music choice. */
export function shows(e: Pick<CalendarEntry, 'kind'>, which: TitlesShown): boolean {
	switch (which) {
		case 'all':
			return true;
		case 'shows':
			return e.kind === 'episode';
		case 'music':
			return e.kind === 'album';
	}
	return e.kind !== 'episode' && e.kind !== 'album';
}

/** entryHref is where an entry leads: an album's own page, else the title. */
export function entryHref(e: Pick<CalendarEntry, 'kind' | 'title_id' | 'part_id'>): string {
	return e.kind === 'album' && e.part_id ? `/library/${e.title_id}/albums/${e.part_id}` : `/library/${e.title_id}`;
}

/** albumHave says how much of an album entry is in the Library. */
export function albumHave(e: Pick<CalendarEntry, 'tracks' | 'have'>): string {
	const n = e.tracks ?? 0;
	const h = e.have ?? 0;
	if (!n) return '';
	if (h === 0) return 'no track in the Library yet';
	return h >= n ? 'every track in the Library' : `${h} of ${n} tracks in the Library`;
}

/** entryDay is the local day an entry falls on: an episode with an air time
 *  on the reader's own day (time zone aware), else its air day; a movie
 *  date on its date. */
export function entryDay(e: Pick<CalendarEntry, 'kind' | 'date' | 'timed' | 'air_day'>): string {
	if (e.kind === 'episode') {
		if (e.timed) return ymd(new Date(e.date));
		return e.air_day ?? dayOf(e.date);
	}
	return dayOf(e.date);
}

/** entryTime is an episode's local air time (21:00), or '' when unknown. */
export function entryTime(e: Pick<CalendarEntry, 'kind' | 'date' | 'timed'>): string {
	if (e.kind !== 'episode' || !e.timed) return '';
	const d = new Date(e.date);
	return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
}

/** episodeCode is S01E02 for an episode entry. */
export function episodeCode(e: Pick<CalendarEntry, 'season' | 'episode'>): string {
	const p = (n = 0) => String(n).padStart(2, '0');
	return `S${p(e.season)}E${p(e.episode)}`;
}

/** entryName is the line an entry shows: the movie, the show and episode,
 *  or the artist and album. */
export function entryName(e: CalendarEntry): string {
	if (e.kind === 'album') return `${e.name} – ${e.album ?? 'Album'}`;
	if (e.kind !== 'episode') return `${e.name}${e.year ? ` (${e.year})` : ''}`;
	return `${e.name} ${episodeCode(e)}${e.episode_name ? ` ${e.episode_name}` : ''}`;
}

/** entryKey tells entries apart in a list. */
export function entryKey(e: CalendarEntry): string {
	if (e.kind === 'album') return `a${e.part_id}`;
	return e.kind === 'episode' ? `e${e.item_id}` : `${e.title_id}${e.kind}`;
}

/** ymd is a calendar day as 2026-10-13, read in local time. */
export function ymd(d: Date): string {
	const p = (n: number) => String(n).padStart(2, '0');
	return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

/** dayOf is the calendar day of an API date (its date part, not shifted by time zone). */
export function dayOf(iso: string): string {
	return iso.slice(0, 10);
}

export interface GridDay {
	day: string; // 2026-10-13
	date: number; // 13
	inMonth: boolean;
	today: boolean;
	items: CalendarEntry[];
}

/** monthRange is the first day shown and the day after the last for a month grid (whole weeks, Monday first). */
export function monthRange(year: number, month: number): { from: string; to: string; start: Date; weeks: number } {
	const first = new Date(year, month, 1);
	const lead = (first.getDay() + 6) % 7; // days before it in its week (Monday first)
	const start = new Date(year, month, 1 - lead);
	const days = new Date(year, month + 1, 0).getDate();
	const weeks = Math.ceil((lead + days) / 7);
	const end = new Date(start.getFullYear(), start.getMonth(), start.getDate() + weeks * 7);
	return { from: ymd(start), to: ymd(end), start, weeks };
}

/** monthGrid lays a month out in weeks of seven days, Monday first, with each day's dates. */
export function monthGrid(year: number, month: number, entries: CalendarEntry[], today = ymd(new Date())): GridDay[][] {
	const { start, weeks } = monthRange(year, month);
	const byDay = new Map<string, CalendarEntry[]>();
	for (const e of entries) {
		const k = entryDay(e);
		byDay.set(k, [...(byDay.get(k) ?? []), e]);
	}
	const out: GridDay[][] = [];
	for (let w = 0; w < weeks; w++) {
		const row: GridDay[] = [];
		for (let i = 0; i < 7; i++) {
			const d = new Date(start.getFullYear(), start.getMonth(), start.getDate() + w * 7 + i);
			const k = ymd(d);
			row.push({ day: k, date: d.getDate(), inMonth: d.getMonth() === month, today: k === today, items: byDay.get(k) ?? [] });
		}
		out.push(row);
	}
	return out;
}

/** byDay groups dates by calendar day for the list, in day order. */
export function byDay(entries: CalendarEntry[]): { day: string; items: CalendarEntry[] }[] {
	const out: { day: string; items: CalendarEntry[] }[] = [];
	const sorted = entries.map((e, i) => ({ e, i, k: entryDay(e) })).sort((a, b) => (a.k === b.k ? a.i - b.i : a.k < b.k ? -1 : 1));
	for (const { e, k } of sorted) {
		const g = out.at(-1);
		if (g && g.day === k) g.items.push(e);
		else out.push({ day: k, items: [e] });
	}
	return out;
}

/** shiftMonth moves a month by n (n may be negative). */
export function shiftMonth(year: number, month: number, n: number): { year: number; month: number } {
	const d = new Date(year, month + n, 1);
	return { year: d.getFullYear(), month: d.getMonth() };
}

/** feedLink is the full feed address for a phone, from the page's origin and the path with the key. */
export function feedLink(origin: string, path: string): string {
	return origin.replace(/\/$/, '') + path;
}

/** webcalLink is the same address for phones that open webcal:// links in their calendar app. */
export function webcalLink(link: string): string {
	return link.replace(/^https?:\/\//, 'webcal://');
}
