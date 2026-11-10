// Calendar: the month grid and list for movie dates and episodes.
import type { CalendarEntry, TitleView } from '../core/types.ts';
import { episodeCode as code } from '../core/episodes.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';

/** Which titles the calendar shows. */
export type TitlesShown = 'all' | 'movies' | 'shows' | 'music';

export interface GridDay {
	day: string;
	date: number;
	inMonth: boolean;
	today: boolean;
	items: CalendarEntry[];
}

/** Big picture cards day by day, or a compact list. */
export type CalendarView = 'cards' | 'list';

/** The status dot on a calendar item: in the Library, missing (its day has passed), or still to come. */
export type ItemDot = 'have' | 'missing' | 'upcoming';

/** A movie's dates and whether a copy is in the Library, read from its title. */
export interface MovieFacts {
	cinema?: string;
	digital?: string;
	physical?: string;
	have: boolean;
}

/** Which dates the calendar shows. Following a title puts its dates on your calendar (asking for a title follows it too). mine: what you follow (the default). everyone: what anyone on this server follows, each row saying who. all: the whole Library. */
export type Follows = 'mine' | 'everyone' | 'library';

/** The List view's banner: the server's banner and logo (stored picks only), when sent. */
export type BannerEntry = CalendarEntry & { banner?: string; logo?: string };

export const kindWord: Record<CalendarEntry['kind'], string> = {
	get cinema() {
		return t('discover.calendar.cinema');
	},
	get digital() {
		return t('discover.calendar.movieRelease');
	},
	get physical() {
		return t('discover.calendar.movieRelease');
	},
	get episode() {
		return t('discover.calendar.episode');
	},
	get album() {
		return t('discover.calendar.albumRelease');
	}
};

/** Whether an entry shows for the movies, shows or music choice. */
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

/** Where an entry leads: an album's own page, else the title. */
export function entryHref(e: Pick<CalendarEntry, 'kind' | 'title_id' | 'part_id'>): string {
	return e.kind === 'album' && e.part_id ? `/library/${e.title_id}/albums/${e.part_id}` : `/library/${e.title_id}`;
}

/** The local day an entry falls on: an episode with an air time on the reader's own day (time zone aware), else its air day; a movie on its date. */
export function entryDay(e: Pick<CalendarEntry, 'kind' | 'date' | 'timed' | 'air_day'>): string {
	if (e.kind === 'episode') {
		if (e.timed) return ymd(new Date(e.date));
		return e.air_day ?? dayOf(e.date);
	}
	return dayOf(e.date);
}

/** S01E02 for an episode entry. */
export function episodeCode(e: Pick<CalendarEntry, 'season' | 'episode'>): string {
	return code(e.season ?? 0, e.episode ?? 0);
}

/** Tells entries apart in a list. */
export function entryKey(e: CalendarEntry): string {
	if (e.kind === 'album') return `a${e.part_id}`;
	return e.kind === 'episode' ? `e${e.item_id}` : `${e.title_id}${e.kind}`;
}

/** A calendar day as YYYY-MM-DD, read in local time. */
export function ymd(d: Date): string {
	const p = (n: number) => String(n).padStart(2, '0');
	return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

/** The calendar day of an API date: its date part, not shifted by time zone. */
export function dayOf(iso: string): string {
	return iso.slice(0, 10);
}

/** The first day shown and the day after the last for a month grid (whole weeks, Monday first). */
export function monthRange(year: number, month: number): { from: string; to: string; start: Date; weeks: number } {
	const first = new Date(year, month, 1);
	const lead = (first.getDay() + 6) % 7; // Days before it in its week (Monday first).
	const start = new Date(year, month, 1 - lead);
	const days = new Date(year, month + 1, 0).getDate();
	const weeks = Math.ceil((lead + days) / 7);
	const end = new Date(start.getFullYear(), start.getMonth(), start.getDate() + weeks * 7);
	return { from: ymd(start), to: ymd(end), start, weeks };
}

/** Lays a month out in weeks of seven days, Monday first, with each day's dates. */
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

/** Groups dates by calendar day for the list, in day order. */
// One order inside every day: movies (In theaters, then Streaming), then TV (All day, then by air time), then music; the name breaks ties.
const kindRank: Record<string, number> = { cinema: 0, digital: 1, physical: 1, episode: 2, album: 3 };

export function byDay(entries: CalendarEntry[]): { day: string; items: CalendarEntry[] }[] {
	const out: { day: string; items: CalendarEntry[] }[] = [];
	const sorted = entries.map((e, i) => ({ e, i, k: entryDay(e) })).sort((a, b) => (a.k === b.k ? dayOrder(a.e, b.e) || a.i - b.i : a.k < b.k ? -1 : 1));
	for (const { e, k } of sorted) {
		const g = out.at(-1);
		if (g && g.day === k) g.items.push(e);
		else out.push({ day: k, items: [e] });
	}
	return out;
}

/** Moves a month by n (n may be negative). */
export function shiftMonth(year: number, month: number, n: number): { year: number; month: number } {
	const d = new Date(year, month + n, 1);
	return { year: d.getFullYear(), month: d.getMonth() };
}

/** The Monday of the week a day falls in (local time). */
export function weekStart(d: Date): Date {
	return new Date(d.getFullYear(), d.getMonth(), d.getDate() - ((d.getDay() + 6) % 7));
}

const viewKey = 'holyarrs.calendar-view';

/** Reads ?view=: cards or list. Old layout links still land somewhere sensible (now or 0, the old page, opens List; 1 to 5 open Cards). */
export function viewFromAddress(v: string | null | undefined): CalendarView | null {
	if (v === 'cards' || v === 'list') return v;
	if (v === 'now' || v === '0') return 'list';
	if (v && /^[1-9]$/.test(v)) return 'cards';
	return null;
}

/** The view picked earlier in this session, if any. */
export function readView(store: Pick<Storage, 'getItem'> | undefined = globalThis.sessionStorage): CalendarView | null {
	try {
		const v = store?.getItem(viewKey);
		return v === 'cards' || v === 'list' ? v : null;
	} catch {
		return null;
	}
}

/** Keeps the view for the session; a private window may refuse and that is fine. */
export function saveView(v: CalendarView, store: Pick<Storage, 'setItem'> | undefined = globalThis.sessionStorage) {
	try {
		store?.setItem(viewKey, v);
	} catch {
		// Not kept; the address still says.
	}
}

/** The view the calendar opens on: the address, else the session's pick, else Cards. */
export function startView(address: string | null | undefined, remembered: CalendarView | null): CalendarView {
	return viewFromAddress(address) ?? remembered ?? 'cards';
}

/** The picture for a calendar entry: the episode still, then the backdrop, then the poster. */
export function entryArt(e: Pick<CalendarEntry, 'still' | 'backdrop' | 'poster'>): string | null {
	return e.still || e.backdrop || e.poster || null;
}

/** Which dot a calendar item shows on a given day. */
export function itemDot(e: CalendarEntry, today = ymd(new Date())): ItemDot {
	const have = e.kind === 'album' ? !!e.tracks && (e.have ?? 0) >= e.tracks : e.state === 'in_library' || !!e.versions?.some((v) => v.state === 'in_library');
	if (have) return 'have';
	return entryDay(e) < today ? 'missing' : 'upcoming';
}

/** Reads a movie title's dates and whether any of its copies is in the Library. */
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

/** Keeps one date per movie: the day it can be watched at home (the earlier of its digital and disc dates), else its cinema date. The title's own dates decide when known, so a cinema date in view is dropped when the title says it is already out. */
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

/** Whether anyone, the reader included, follows an entry's title. */
export function followedByAnyone(e: Pick<CalendarEntry, 'following' | 'followers' | 'followers_hidden'>): boolean {
	return !!e.following || (e.followers?.length ?? 0) > 0 || (e.followers_hidden ?? 0) > 0;
}

/** Whether an entry shows for the Follows choice. */
export function followsOk(e: Pick<CalendarEntry, 'following' | 'followers' | 'followers_hidden'>, f: Follows): boolean {
	if (f === 'library') return true;
	if (f === 'mine') return !!e.following;
	return followedByAnyone(e);
}

/** Who follows an entry's title: "You, Ana and 2 others". */
export function followerNames(e: Pick<CalendarEntry, 'followers' | 'followers_hidden'>): string {
	const names = (e.followers ?? []).map((p) => (p.me ? t('discover.calendar.you') : p.name));
	const hidden = e.followers_hidden ?? 0;
	if (hidden > 0) names.push(hidden === 1 ? (names.length ? t('discover.calendar.oneOther') : t('discover.calendar.onePerson')) : names.length ? t('discover.calendar.others', { n: hidden }) : t('discover.calendar.people', { n: hidden }));
	if (names.length <= 1) return names[0] ?? '';
	return t('discover.calendar.nameList', { first: names.slice(0, -1).join(', '), last: names.at(-1) ?? '' });
}

/** An entry after the reader follows (on) or stops following its title. */
export function withFollow<T extends CalendarEntry>(e: T, on: boolean, me: { name: string; avatar?: string | null }): T {
	const mine = e.followers?.find((p) => p.me);
	const others = (e.followers ?? []).filter((p) => !p.me);
	const self = { id: mine?.id ?? 0, name: me.name, avatar: me.avatar ?? undefined, me: true };
	return { ...e, following: on, followers: on ? [...others, self] : others };
}

const followsKey = 'holyarrs.calendar-follows';

/** The Follows choice picked earlier in this session, if any. */
export function readFollows(store: Pick<Storage, 'getItem'> | undefined = globalThis.sessionStorage): Follows | null {
	try {
		const v = store?.getItem(followsKey);
		return v === 'mine' || v === 'everyone' || v === 'library' ? v : null;
	} catch {
		return null;
	}
}

/** Keeps the Follows choice for the session; a private window may refuse and that is fine. */
export function saveFollows(v: Follows, store: Pick<Storage, 'setItem'> | undefined = globalThis.sessionStorage) {
	try {
		store?.setItem(followsKey, v);
	} catch {
		// Not kept for the session.
	}
}

/** The choice the calendar opens on: the session's pick, else My follows. Someone who follows nothing opens on the Whole Library, and Everyone's follows needs more than one person on the server. */
export function startFollows(remembered: Follows | null, myFollows: number, people: number): Follows {
	let f: Follows = remembered ?? (myFollows > 0 ? 'mine' : 'library');
	if (f === 'everyone' && people < 2) f = myFollows > 0 ? 'mine' : 'library';
	return f;
}

/** Moves a day by n days (local time, midnight). */
export function addDays(d: Date, n: number): Date {
	return new Date(d.getFullYear(), d.getMonth(), d.getDate() + n);
}

/** Splits a calendar query into pieces the server takes (at most `max` days each). */
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

/** Reads ?titles=: all (once called everything), movies, shows or music. */
export function titlesFromAddress(v: string | null | undefined): TitlesShown {
	return v === 'movies' || v === 'shows' || v === 'music' ? v : 'all';
}

/** A List row's wide picture, best first (the row moves to the next when one fails to load), and the logo over it. Movies and shows use the textless banner under the logo, else the title backdrop, else an episode still. */
export function bannerArt(e: BannerEntry): { images: string[]; logo: string | null } {
	const list = e.kind === 'album' ? [e.banner, e.backdrop, e.poster] : [e.banner, e.backdrop, e.still, e.poster];
	return { images: [...new Set(list.filter((x): x is string => !!x))], logo: e.logo || null };
}

const MONTH_KEY = 'holyarrs.calendar.month-hidden';

/** Whether the small month was hidden last time (false when storage is unavailable). */
export function readMonthHidden(): boolean {
	try {
		return globalThis.localStorage?.getItem(MONTH_KEY) === '1';
	} catch {
		return false;
	}
}

/** Remembers the small month's choice; storage errors are ignored. */
export function saveMonthHidden(hidden: boolean): void {
	try {
		globalThis.localStorage?.setItem(MONTH_KEY, hidden ? '1' : '0');
	} catch {
		/* Private window or blocked storage: the choice lasts for this page only. */
	}
}

function dayOrder(a: CalendarEntry, b: CalendarEntry): number {
	const k = (kindRank[a.kind] ?? 9) - (kindRank[b.kind] ?? 9);
	if (k) return k;
	if (a.kind === 'episode') {
		const at = a.timed ? Date.parse(a.date) : -Infinity;
		const bt = b.timed ? Date.parse(b.date) : -Infinity;
		if (at !== bt) return at < bt ? -1 : 1;
	}
	return (a.name ?? '').localeCompare(b.name ?? '') || (a.season ?? 0) - (b.season ?? 0) || (a.episode ?? 0) - (b.episode ?? 0);
}
