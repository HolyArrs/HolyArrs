// The calendar's words and groupings: card badges, facts lines, day names, month dividers and the cards a busy day folds into.
import { byDay, entryDay, kindWord } from './calendar.ts';
import type { CalendarEntry } from '../core/types.ts';
import { t, appLocale } from '#lib/core/i18n/i18n.svelte.ts';

/** One card's dates: one, or several of one show (or one movie) on the same day. */
export interface CalGroup {
	key: string;
	day: string;
	lead: CalendarEntry;
	entries: CalendarEntry[];
}

const at = (day: string) => new Date(day + 'T00:00:00');
const daysBetween = (a: string, b: string) => Math.round((at(b).getTime() - at(a).getTime()) / 86400000);

/** A day heading: Today, Tomorrow, Yesterday, else "Thursday 29 Oct". */
export function dayName(day: string, today: string): string {
	const n = daysBetween(today, day);
	if (n === 0) return t('discover.calendarGroups.today');
	if (n === 1) return t('discover.calendarGroups.tomorrow');
	if (n === -1) return t('discover.calendarGroups.yesterday');
	return at(day).toLocaleDateString(appLocale(), { weekday: 'long', day: 'numeric', month: 'short' });
}

/** The short date on a card: Today, Tomorrow, else "Thu 29". */
export function dayChip(day: string, today: string): string {
	const n = daysBetween(today, day);
	if (n === 0) return t('discover.calendarGroups.today');
	if (n === 1) return t('discover.calendarGroups.tomorrow');
	const d = at(day);
	return `${d.toLocaleDateString(appLocale(), { weekday: 'short' })} ${d.getDate()}`;
}


/** The badge on a card: New episode, Season premiere, In cinemas, New album. */
export function entryTag(e: CalendarEntry): string {
	if (e.kind === 'episode') {
		if (e.episode === 1) return e.season === 1 ? t('discover.calendarGroups.showPremiere') : t('discover.calendarGroups.seasonPremiere');
		return t('discover.calendarGroups.newEpisode');
	}
	if (e.kind === 'album') {
		const k = (e.album_type ?? '').toLowerCase();
		return k === 'single' ? t('discover.calendarGroups.newSingle') : k === 'ep' ? t('discover.calendarGroups.newEp') : t('discover.calendarGroups.newAlbum');
	}
	// A movie shows one date: the day it is out to watch, or its cinema date as a small tag.
	return e.kind === 'cinema' ? kindWord.cinema : t('discover.calendarGroups.newMovie');
}

/** The big name on a card: the show, movie or album. */
export function entryTitle(e: CalendarEntry): string {
	return e.kind === 'album' ? (e.album ?? e.name) : e.name;
}

const movieKinds = new Set<CalendarEntry['kind']>(['cinema', 'digital', 'physical']);
const shortKind = (): Record<string, string> => ({ cinema: t('discover.calendarGroups.cinemas'), digital: t('discover.calendarGroups.digital'), physical: t('discover.calendarGroups.disc') });

/** Splits day groups by month for dividers ("November 2026"). */
export function monthGroups(entries: CalendarEntry[]): { month: string; name: string; days: { day: string; items: CalendarEntry[] }[] }[] {
	const out: { month: string; name: string; days: { day: string; items: CalendarEntry[] }[] }[] = [];
	for (const d of byDay(entries)) {
		const m = d.day.slice(0, 7);
		const g = out.at(-1);
		if (g && g.month === m) g.days.push(d);
		else out.push({ month: m, name: at(d.day).toLocaleDateString(appLocale(), { month: 'long', year: 'numeric' }), days: [d] });
	}
	return out;
}

/** A day's count: "1 coming out", "10 coming out". */
export function thingCount(n: number): string {
	return t('discover.calendarGroups.comingOut', { n });
}

/** Folds dates into cards. A show's episodes on one day become one card ("3 episodes"), as do one movie's dates on one day (digital and disc); albums stay one card each. Order follows each card's first date. */
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

/** A card's badge: "3 episodes", "Digital and disc", else the date's own tag. */
export function groupTag(g: CalGroup): string {
	const n = g.entries.length;
	if (n === 1) return entryTag(g.lead);
	if (g.lead.kind === 'episode') return t('discover.calendarGroups.episodes', { n });
	if (g.lead.kind === 'album') return t('discover.calendarGroups.albums', { n });
	const kinds = shortKind();
	const words = [...new Set(g.entries.map((e) => kinds[e.kind]))].map((w, i) => (i ? w.toLowerCase() : w));
	return words.length > 1 ? t('discover.calendarGroups.list', { first: words.slice(0, -1).join(', '), last: words.at(-1) ?? '' }) : words[0];
}

