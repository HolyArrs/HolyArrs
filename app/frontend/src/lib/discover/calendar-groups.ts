// The calendar's words and groupings: card badges, facts lines, day names,
// month dividers and the cards a busy day folds into.
import type { CalendarEntry } from '../core/types.ts';
import { byDay, entryDay, entryTime, episodeCode, kindWord } from './calendar.ts';

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
	// A movie shows one date: the day it is out to watch, or its cinema date as a small tag.
	return e.kind === 'cinema' ? kindWord.cinema : 'New movie';
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

/** entryFacts is the small facts line: "S23E03 · 20:00 · ABC", "EP · 6 tracks", "Movie release". */
export function entryFacts(e: CalendarEntry): string {
	if (e.kind === 'episode') return [episodeCode(e), entryTime(e), e.network].filter(Boolean).join(' · ');
	if (e.kind === 'album') return [albumWord(e.album_type) === 'EP' ? 'EP' : albumWord(e.album_type).replace(/^./, (c) => c.toUpperCase()), e.tracks ? `${e.tracks} tracks` : ''].filter(Boolean).join(' · ');
	return kindWord[e.kind];
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

/** thingCount is a day's count: "1 coming out", "10 coming out". */
export function thingCount(n: number): string {
	return `${n} coming out`;
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
