// Calendar: the month grid and list for movie dates and episodes, and the
// feed link.
import type { CalendarEntry, TitleView } from './types.ts';

export const kindWord: Record<CalendarEntry['kind'], string> = {
	cinema: 'In cinemas',
	digital: 'Movie release',
	physical: 'Movie release',
	episode: 'Episode',
	album: 'Album release'
};
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
	if (h === 0) return 'Not in your Library yet';
	return h >= n ? 'In your Library' : `${h} of ${n} songs in your Library`;
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

/** The calendar's two views: big picture cards day by day, or a compact list. */
export type CalendarView = 'cards' | 'list';

/** weekStart is the Monday of the week a day falls in (local time). */
export function weekStart(d: Date): Date {
	return new Date(d.getFullYear(), d.getMonth(), d.getDate() - ((d.getDay() + 6) % 7));
}

/** shiftWeek moves a week start by n weeks. */
export function shiftWeek(start: Date, n: number): Date {
	return new Date(start.getFullYear(), start.getMonth(), start.getDate() + n * 7);
}

const viewKey = 'holyarrs.calendar-view';

/**
 * viewFromAddress reads ?view=: cards or list; the old layout links still
 * land somewhere sensible (now or 0, the old page, opens List; 1 to 5 open Cards).
 */
export function viewFromAddress(v: string | null | undefined): CalendarView | null {
	if (v === 'cards' || v === 'list') return v;
	if (v === 'now' || v === '0') return 'list';
	if (v && /^[1-9]$/.test(v)) return 'cards';
	return null;
}

/** readView is the view picked earlier in this session, if any. */
export function readView(store: Pick<Storage, 'getItem'> | undefined = globalThis.sessionStorage): CalendarView | null {
	try {
		const v = store?.getItem(viewKey);
		return v === 'cards' || v === 'list' ? v : null;
	} catch {
		return null;
	}
}

/** saveView keeps the view picked for the session; a private window may refuse and that is fine. */
export function saveView(v: CalendarView, store: Pick<Storage, 'setItem'> | undefined = globalThis.sessionStorage) {
	try {
		store?.setItem(viewKey, v);
	} catch {
		// Not kept; the address still says.
	}
}

/** startView is the view the calendar opens on: the address, else the session's pick, else Cards. */
export function startView(address: string | null | undefined, remembered: CalendarView | null): CalendarView {
	return viewFromAddress(address) ?? remembered ?? 'cards';
}

/** entryArt is the picture for a calendar entry: the episode still, then the backdrop, then the poster. */
export function entryArt(e: Pick<CalendarEntry, 'still' | 'backdrop' | 'poster'>): string | null {
	return e.still || e.backdrop || e.poster || null;
}

/** entryIcon is the kind icon an entry shows when it has no picture. */
export function entryIcon(kind: CalendarEntry['kind']): 'tv' | 'music' | 'film' {
	return kind === 'episode' ? 'tv' : kind === 'album' ? 'music' : 'film';
}

/** The status dot on a calendar item: in the Library, missing (its day has passed), or still to come. */
export type ItemDot = 'have' | 'missing' | 'upcoming';

/** itemDot says which dot a calendar item shows on a given day. */
export function itemDot(e: CalendarEntry, today = ymd(new Date())): ItemDot {
	const have = e.kind === 'album' ? !!e.tracks && (e.have ?? 0) >= e.tracks : e.state === 'in_library' || !!e.versions?.some((v) => v.state === 'in_library');
	if (have) return 'have';
	return entryDay(e) < today ? 'missing' : 'upcoming';
}

/** A movie's dates and whether a copy is in the Library, read from its title. */
export interface MovieFacts {
	cinema?: string;
	digital?: string;
	physical?: string;
	have: boolean;
}

/** movieFacts reads a movie title's dates and whether any of its copies is in the Library. */
export function movieFacts(t: Pick<TitleView, 'cinema_date' | 'digital_date' | 'physical_date' | 'slots'>): MovieFacts {
	return {
		cinema: t.cinema_date ? dayOf(t.cinema_date) : undefined,
		digital: t.digital_date ? dayOf(t.digital_date) : undefined,
		physical: t.physical_date ? dayOf(t.physical_date) : undefined,
		have: !!t.slots?.some((s) => s.state === 'in_library')
	};
}

const homeKinds = new Set<CalendarEntry['kind']>(['digital', 'physical']);
const isMovie = (e: Pick<CalendarEntry, 'kind'>) => e.kind === 'cinema' || homeKinds.has(e.kind);

/**
 * oneMovieDate keeps one date per movie: the day it can be watched at home
 * (the earlier of its digital and disc dates), else its cinema date. The
 * title's own dates decide when known, so a cinema date in view is dropped
 * when the home date is later; without them the loaded dates decide. A movie
 * with a copy in the Library is marked so (the server sends no state for movies).
 */
export function oneMovieDate(entries: CalendarEntry[], movies: Map<number, MovieFacts> = new Map()): CalendarEntry[] {
	const home = new Map<number, string>();
	for (const e of entries) {
		if (!homeKinds.has(e.kind)) continue;
		const d = dayOf(e.date);
		const was = home.get(e.title_id);
		if (!was || d < was) home.set(e.title_id, d);
	}
	const kept = new Set<number>();
	const out: CalendarEntry[] = [];
	for (const e of entries) {
		if (!isMovie(e)) {
			out.push(e);
			continue;
		}
		if (kept.has(e.title_id)) continue;
		const f = movies.get(e.title_id);
		const homeDays = f ? [f.digital, f.physical].filter((d): d is string => !!d).sort() : [];
		const homeDay = f ? homeDays[0] : home.get(e.title_id);
		const keep = homeDay ? homeKinds.has(e.kind) && dayOf(e.date) === homeDay : e.kind === 'cinema';
		if (!keep) continue;
		kept.add(e.title_id);
		out.push(f?.have ? { ...e, state: 'in_library' } : e);
	}
	return out;
}

/**
 * Which dates the calendar shows. Following a title puts its dates on your
 * calendar (asking for a title follows it too):
 * - mine: what you follow (the default),
 * - everyone: what anyone on this server follows, each row saying who,
 * - library: every date in the Library, followed or not.
 */
export type Follows = 'mine' | 'everyone' | 'library';

/** followedByAnyone says whether anyone (the reader included) follows an entry's title. */
export function followedByAnyone(e: Pick<CalendarEntry, 'following' | 'followers' | 'followers_hidden'>): boolean {
	return !!e.following || (e.followers?.length ?? 0) > 0 || (e.followers_hidden ?? 0) > 0;
}

/** followsOk says whether an entry shows for the Follows choice. */
export function followsOk(e: Pick<CalendarEntry, 'following' | 'followers' | 'followers_hidden'>, f: Follows): boolean {
	if (f === 'library') return true;
	if (f === 'mine') return !!e.following;
	return followedByAnyone(e);
}

/** followerNames says who follows an entry's title: "You, Ana and 2 others". */
export function followerNames(e: Pick<CalendarEntry, 'followers' | 'followers_hidden'>): string {
	const names = (e.followers ?? []).map((p) => (p.me ? 'You' : p.name));
	const hidden = e.followers_hidden ?? 0;
	if (hidden > 0) names.push(hidden === 1 ? (names.length ? '1 other' : '1 person') : `${hidden} ${names.length ? 'others' : 'people'}`);
	if (names.length <= 1) return names[0] ?? '';
	return `${names.slice(0, -1).join(', ')} and ${names.at(-1)}`;
}

/** withFollow is an entry after the reader follows (on) or stops following its title. */
export function withFollow<T extends CalendarEntry>(e: T, on: boolean, me: { name: string; avatar?: string | null }): T {
	const mine = e.followers?.find((p) => p.me);
	const others = (e.followers ?? []).filter((p) => !p.me);
	const self = { id: mine?.id ?? 0, name: me.name, avatar: me.avatar ?? undefined, me: true };
	return { ...e, following: on, followers: on ? [...others, self] : others };
}

const followsKey = 'holyarrs.calendar-follows';

/** readFollows is the Follows choice picked earlier in this session, if any. */
export function readFollows(store: Pick<Storage, 'getItem'> | undefined = globalThis.sessionStorage): Follows | null {
	try {
		const v = store?.getItem(followsKey);
		return v === 'mine' || v === 'everyone' || v === 'library' ? v : null;
	} catch {
		return null;
	}
}

/** saveFollows keeps the Follows choice for the session; a private window may refuse and that is fine. */
export function saveFollows(v: Follows, store: Pick<Storage, 'setItem'> | undefined = globalThis.sessionStorage) {
	try {
		store?.setItem(followsKey, v);
	} catch {
		// Not kept for the session.
	}
}

/**
 * startFollows is the choice the calendar opens on: the session's pick, else
 * My follows; someone who follows nothing opens on the Whole Library, and
 * Everyone's follows needs more than one person on the server.
 */
export function startFollows(remembered: Follows | null, myFollows: number, people: number): Follows {
	let f: Follows = remembered ?? (myFollows > 0 ? 'mine' : 'library');
	if (f === 'everyone' && people < 2) f = myFollows > 0 ? 'mine' : 'library';
	return f;
}

/** addDays moves a day by n days (local time, midnight). */
export function addDays(d: Date, n: number): Date {
	return new Date(d.getFullYear(), d.getMonth(), d.getDate() + n);
}

/** rangeChunks splits a calendar query into pieces the server takes (at most `max` days each). */
export function rangeChunks(from: string, to: string, max = 360): { from: string; to: string }[] {
	const out: { from: string; to: string }[] = [];
	let a = new Date(from + 'T00:00:00');
	const end = new Date(to + 'T00:00:00');
	while (a < end) {
		const b = addDays(a, max) < end ? addDays(a, max) : end;
		out.push({ from: ymd(a), to: ymd(b) });
		a = b;
	}
	return out;
}

/** titlesFromAddress reads ?titles=: all (once called everything), movies, shows or music. */
export function titlesFromAddress(v: string | null | undefined): TitlesShown {
	return v === 'movies' || v === 'shows' || v === 'music' ? v : 'all';
}
