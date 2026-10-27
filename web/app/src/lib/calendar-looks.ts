// The calendar's design variants (?view=1…5): the words and groupings they
// share. Every variant draws from the same loaded dates as today's page.
import type { CalendarEntry } from './types.ts';
import { byDay, entryDay, entryTime, episodeCode, kindWord, weekStart, ymd } from './calendar.ts';

/** The variants on the switch; 0 is today's page. */
export const calendarLooks = [
	{ value: 0, label: 'Now' },
	{ value: 1, label: '1 Up Next' },
	{ value: 2, label: '2 Coming Soon' },
	{ value: 3, label: '3 Week strip' },
	{ value: 4, label: '4 Shelves' },
	{ value: 5, label: '5 Month' }
];

const at = (day: string) => new Date(day + 'T00:00:00');
const daysBetween = (a: string, b: string) => Math.round((at(b).getTime() - at(a).getTime()) / 86400000);

/** dayName is a day heading: Today, Tomorrow, Yesterday, else "Thursday 29 Oct". */
export function dayName(day: string, today: string): string {
	const n = daysBetween(today, day);
	if (n === 0) return 'Today';
	if (n === 1) return 'Tomorrow';
	if (n === -1) return 'Yesterday';
	return at(day).toLocaleDateString(undefined, { weekday: 'long', day: 'numeric', month: 'short' });
}

/** dayChip is the short date on a card: Today, Tomorrow, else "Thu 29". */
export function dayChip(day: string, today: string): string {
	const n = daysBetween(today, day);
	if (n === 0) return 'Today';
	if (n === 1) return 'Tomorrow';
	const d = at(day);
	return `${d.toLocaleDateString(undefined, { weekday: 'short' })} ${d.getDate()}`;
}

/** A generic episode name ("Episode 4", "TBA") says nothing more than the number. */
export function plainEpisodeName(name?: string): boolean {
	return !name || /^(episode\s*\d+|tba|tbd)$/i.test(name.trim());
}

const albumWord = (t?: string) => {
	const k = (t ?? '').toLowerCase();
	return k === 'single' ? 'single' : k === 'ep' ? 'EP' : 'album';
};

/** entryTag is the badge on a card: New episode, Season premiere, In cinemas, New album… */
export function entryTag(e: CalendarEntry): string {
	if (e.kind === 'episode') {
		if (e.episode === 1) return e.season === 1 ? 'Show premiere' : 'Season premiere';
		return 'New episode';
	}
	if (e.kind === 'album') return `New ${albumWord(e.album_type)}`;
	return kindWord[e.kind];
}

/** entryTitle is the big name on a card: the show, the movie, or the album. */
export function entryTitle(e: CalendarEntry): string {
	return e.kind === 'album' ? (e.album ?? e.name) : e.name;
}

/** entryLine is the line under the name: the episode's own name, the artist, or the movie's year. */
export function entryLine(e: CalendarEntry): string {
	if (e.kind === 'episode') return plainEpisodeName(e.episode_name) ? '' : (e.episode_name ?? '');
	if (e.kind === 'album') return e.name;
	return e.year ? String(e.year) : '';
}

/** entryFacts is the small facts line: "S23E03 · 20:00 · ABC", "EP · 6 tracks", "Digital release". */
export function entryFacts(e: CalendarEntry): string {
	if (e.kind === 'episode') return [episodeCode(e), entryTime(e), e.network].filter(Boolean).join(' · ');
	if (e.kind === 'album') return [albumWord(e.album_type) === 'EP' ? 'EP' : albumWord(e.album_type).replace(/^./, (c) => c.toUpperCase()), e.tracks ? `${e.tracks} tracks` : ''].filter(Boolean).join(' · ');
	return kindWord[e.kind];
}

/** entryKindArt says which card shape fits: wide stills for episodes and cinema, posters for home release, squares for albums. */
export function entryShape(e: Pick<CalendarEntry, 'kind'>): 'wide' | 'poster' | 'square' {
	if (e.kind === 'album') return 'square';
	if (e.kind === 'digital' || e.kind === 'physical') return 'poster';
	return 'wide';
}

const movieKinds = new Set<CalendarEntry['kind']>(['cinema', 'digital', 'physical']);
const shortKind: Record<string, string> = { cinema: 'Cinemas', digital: 'Digital', physical: 'Disc' };

/** movieDates lists a movie's other dates among the loaded ones: "Digital 3 Mar · Disc 2 Apr". */
export function movieDates(e: CalendarEntry, all: CalendarEntry[]): string {
	if (!movieKinds.has(e.kind)) return '';
	return all
		.filter((x) => x.title_id === e.title_id && x.kind !== e.kind && movieKinds.has(x.kind))
		.sort((a, b) => (entryDay(a) < entryDay(b) ? -1 : 1))
		.map((x) => `${shortKind[x.kind]} ${at(entryDay(x)).toLocaleDateString(undefined, { day: 'numeric', month: 'short' })}`)
		.join(' · ');
}

/** monthGroups splits day groups by month for dividers ("November 2026"). */
export function monthGroups(entries: CalendarEntry[]): { month: string; name: string; days: { day: string; items: CalendarEntry[] }[] }[] {
	const out: { month: string; name: string; days: { day: string; items: CalendarEntry[] }[] }[] = [];
	for (const d of byDay(entries)) {
		const m = d.day.slice(0, 7);
		const g = out.at(-1);
		if (g && g.month === m) g.days.push(d);
		else out.push({ month: m, name: at(d.day).toLocaleDateString(undefined, { month: 'long', year: 'numeric' }), days: [d] });
	}
	return out;
}

export interface StripDay {
	day: string;
	date: number;
	dow: string;
	month: string;
	today: boolean;
	past: boolean;
	/** The first day of a month in the strip carries its month name. */
	first: boolean;
	items: CalendarEntry[];
}

/** stripDays lays out n days from start for the date strip, with each day's items. */
export function stripDays(start: Date, n: number, entries: CalendarEntry[], today: string): StripDay[] {
	const by = new Map(byDay(entries).map((g) => [g.day, g.items]));
	const out: StripDay[] = [];
	for (let i = 0; i < n; i++) {
		const d = new Date(start.getFullYear(), start.getMonth(), start.getDate() + i);
		const k = ymd(d);
		out.push({
			day: k,
			date: d.getDate(),
			dow: d.toLocaleDateString(undefined, { weekday: 'short' }),
			month: d.toLocaleDateString(undefined, { month: 'short' }),
			today: k === today,
			past: k < today,
			first: i === 0 || d.getDate() === 1,
			items: by.get(k) ?? []
		});
	}
	return out;
}

/** nextBusy is the first day after `day` that has items, if any. */
export function nextBusy(day: string, days: { day: string; items: unknown[] }[]): string | null {
	return days.find((d) => d.day > day && d.items.length)?.day ?? null;
}

/** startDay is the strip's first pick: today when shown, else the first day with items, else the first day. */
export function startDay(days: StripDay[]): string | null {
	return days.find((d) => d.today)?.day ?? days.find((d) => d.items.length)?.day ?? days[0]?.day ?? null;
}

export interface Shelf {
	key: string;
	title: string;
	shape: 'wide' | 'poster' | 'square';
	items: CalendarEntry[];
}

/**
 * shelves groups dates into Netflix-style rows: episodes and cinema dates by
 * when (Today, This week, Next week, Later this month, then by month), then
 * Coming to digital, Coming on disc and New albums. Empty rows are left out.
 */
export function shelves(entries: CalendarEntry[], today: string): Shelf[] {
	const rows = new Map<string, Shelf & { order: number }>();
	const thisWeek = ymd(weekStart(at(today)));
	const add = (key: string, title: string, order: number, shape: Shelf['shape'], e: CalendarEntry) => {
		const r = rows.get(key) ?? { key, title, order, shape, items: [] };
		r.items.push(e);
		rows.set(key, r);
	};
	const sorted = byDay(entries).flatMap((g) => g.items);
	for (const e of sorted) {
		const day = entryDay(e);
		if (e.kind === 'album') add('albums', 'New albums', 900, 'square', e);
		else if (e.kind === 'digital') add('digital', 'Coming to digital', 800, 'poster', e);
		else if (e.kind === 'physical') add('disc', 'Coming on disc', 850, 'poster', e);
		else {
			const weeks = Math.round(daysBetween(thisWeek, ymd(weekStart(at(day)))) / 7);
			if (day === today) add('today', 'Today', 0, 'wide', e);
			else if (day < today) add('out', weeks === 0 ? 'Earlier this week' : 'Already out', 700, 'wide', e);
			else if (weeks === 0) add('week', 'This week', 1, 'wide', e);
			else if (weeks === 1) add('next', 'Next week', 2, 'wide', e);
			else if (day.slice(0, 7) === today.slice(0, 7)) add('month', 'Later this month', 3, 'wide', e);
			else {
				const m = day.slice(0, 7);
				const months = (Number(m.slice(0, 4)) - Number(today.slice(0, 4))) * 12 + Number(m.slice(5, 7)) - Number(today.slice(5, 7));
				add(`m${m}`, `In ${at(day).toLocaleDateString(undefined, { month: 'long' })}`, 10 + months, 'wide', e);
			}
		}
	}
	return [...rows.values()].sort((a, b) => a.order - b.order).map(({ order: _o, ...r }) => r);
}

/** dayDots is up to three kind dots for a day (episode, movie, album), in that order. */
export function dayDots(items: Pick<CalendarEntry, 'kind'>[]): ('episode' | 'movie' | 'album')[] {
	const has = (k: 'episode' | 'movie' | 'album') => items.some((e) => (k === 'movie' ? movieKinds.has(e.kind) : e.kind === k));
	return (['episode', 'movie', 'album'] as const).filter(has);
}

/** How far Coming Soon (2) looks at once: the day-by-day list, one week, or a month grid. */
export type Span = 'day' | 'week' | 'month';

/** readSpan reads ?span=; anything else is the day-by-day list. */
export function readSpan(v: string | null | undefined): Span {
	return v === 'week' || v === 'month' ? v : 'day';
}

/** spanName is the heading between the arrows: "Oct 26 – Nov 1, 2026" for a week, "October 2026" for a month. */
export function spanName(span: 'week' | 'month', start: Date, year: number, month: number): string {
	if (span === 'month') return new Date(year, month, 1).toLocaleDateString(undefined, { month: 'long', year: 'numeric' });
	const end = new Date(start.getFullYear(), start.getMonth(), start.getDate() + 6);
	return `${start.toLocaleDateString(undefined, { day: 'numeric', month: 'short' })} – ${end.toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' })}`;
}

/**
 * fitThumbs is how many pictures of the given widths fit side by side in
 * `room` pixels with `gap` between them. When not all fit, room is kept for a
 * "+N" chip `more` pixels wide, so the count shown plus the chip always fit.
 */
export function fitThumbs(widths: number[], room: number, gap: number, more: number): number {
	let used = 0;
	let n = 0;
	for (const w of widths) {
		const next = used + (n ? gap : 0) + w;
		if (next > room) break;
		used = next;
		n++;
	}
	if (n === widths.length) return n;
	while (n > 0) {
		let total = 0;
		for (let i = 0; i < n; i++) total += widths[i] + (i ? gap : 0);
		if (total + gap + more <= room) break;
		n--;
	}
	return n;
}

/** thingCount is a day's or a week's count: "1 thing", "10 things". */
export function thingCount(n: number): string {
	return n === 1 ? '1 thing' : `${n} things`;
}

/** A card's worth of dates: one, or several of one show (or one movie) on the same day. */
export interface CalGroup {
	key: string;
	day: string;
	lead: CalendarEntry;
	entries: CalendarEntry[];
}

/**
 * groupDates folds dates into cards: a show's episodes on one day become one
 * card ("3 episodes"), as do one movie's dates on one day (digital and disc);
 * albums stay one card each. Order is the order of each card's first date.
 */
export function groupDates(entries: CalendarEntry[]): CalGroup[] {
	const out = new Map<string, CalGroup>();
	for (const e of entries) {
		const day = entryDay(e);
		const who = e.kind === 'album' ? `al${e.part_id ?? e.album ?? e.name}` : e.kind === 'episode' ? `ep${e.title_id}` : `mv${e.title_id}`;
		const key = `${day}:${who}`;
		const g = out.get(key);
		if (g) g.entries.push(e);
		else out.set(key, { key, day, lead: e, entries: [e] });
	}
	return [...out.values()];
}

const p2 = (n = 0) => String(n).padStart(2, '0');

/** groupTag is a card's badge: "3 episodes", "Digital and disc", else the date's own tag. */
export function groupTag(g: CalGroup): string {
	const n = g.entries.length;
	if (n === 1) return entryTag(g.lead);
	if (g.lead.kind === 'episode') return `${n} episodes`;
	if (g.lead.kind === 'album') return `${n} albums`;
	const words = [...new Set(g.entries.map((e) => shortKind[e.kind]))].map((w, i) => (i ? w.toLowerCase() : w));
	return words.length > 1 ? `${words.slice(0, -1).join(', ')} and ${words.at(-1)}` : words[0];
}

/** groupFacts is a card's facts line; several episodes show their span: "S39E194–E196 · 19:00 · Seven". */
export function groupFacts(g: CalGroup): string {
	if (g.entries.length === 1 || g.lead.kind !== 'episode') return entryFacts(g.lead);
	const eps = [...g.entries].sort((a, b) => (a.season ?? 0) - (b.season ?? 0) || (a.episode ?? 0) - (b.episode ?? 0));
	const first = eps[0];
	const last = eps.at(-1)!;
	const span = first.season === last.season ? `${episodeCode(first)}–E${p2(last.episode)}` : `${episodeCode(first)}–${episodeCode(last)}`;
	return [span, entryTime(first), first.network].filter(Boolean).join(' · ');
}

/** weekTitle heads a week's row: "This week · Oct 26 – Nov 1", "Next week · …", else the dates alone. */
export function weekTitle(start: Date, today: string): string {
	const end = new Date(start.getFullYear(), start.getMonth(), start.getDate() + 6);
	const f = (d: Date) => d.toLocaleDateString(undefined, { day: 'numeric', month: 'short' });
	const range = `${f(start)} – ${f(end)}`;
	const n = Math.round(daysBetween(ymd(weekStart(at(today))), ymd(weekStart(start))) / 7);
	if (n === 0) return `This week · ${range}`;
	if (n === 1) return `Next week · ${range}`;
	if (n === -1) return `Last week · ${range}`;
	return range;
}
