// The Stage views of the Approvals page Luis compares (?stage=1…5), and
// the rules they share: what an ask is for in one line
// (with season pills), whether part of it is not out yet, the status tabs,
// and the remembered poster size and side panel.

import type { Ask } from '../types.ts';
import type { AskTab } from '../wishlist.ts';

/** The Stage views on the bottom bar (?stage=1…5): the picked ask large, the list roomier in each. */

/** The status tabs, in the words the app uses (Downloads-style, each with its count). */
export const inboxTabs: { tab: AskTab; label: string }[] = [
	{ tab: 'waiting', label: 'Waiting' },
	{ tab: 'approved', label: 'Approved' },
	{ tab: 'declined', label: 'Turned down' },
	{ tab: 'done', label: 'Done' },
	{ tab: 'failed', label: 'Failed' }
];

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

/** readInfo picks the summary's parts out of a /metadata answer, tolerating missing fields. */
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

/** runtimeShort says a runtime the compact way: "2h 10m", "48m", "2h". */
export function runtimeShort(min: number | undefined): string {
	if (!min) return '';
	const h = Math.floor(min / 60);
	const m = min % 60;
	return h ? (m ? `${h}h ${m}m` : `${h}h`) : `${m}m`;
}

const day = (iso: string) => iso.slice(0, 10);
/** shortDate says "Mar 3", or "Mar 3, 2027" outside this year. */
export function shortDate(iso: string, now = new Date()): string {
	const [y, m, d] = day(iso).split('-').map(Number);
	if (!y || !m || !d) return '';
	const t = new Date(Date.UTC(y, m - 1, d));
	const opts: Intl.DateTimeFormatOptions = { month: 'short', day: 'numeric', timeZone: 'UTC' };
	if (y !== now.getFullYear()) opts.year = 'numeric';
	return t.toLocaleDateString('en-US', opts);
}

/** movieRelease says whether a movie is out: out now, in cinemas only, or not out yet (with the cinema date). */
export function movieRelease(info: Pick<AskInfo, 'dates' | 'status'> | null | undefined, now = new Date()): { words: string; notOut: string } {
	if (!info) return { words: '', notOut: '' };
	const today = now.toISOString().slice(0, 10);
	const { theaters, digital, physical } = info.dates;
	const home = [digital, physical].filter(Boolean).map((x) => day(x)).sort()[0];
	if (home && home <= today) return { words: 'out now', notOut: '' };
	if (theaters && day(theaters) <= today) return { words: 'in cinemas only', notOut: '' };
	if (theaters) return { words: `in cinemas ${shortDate(theaters, now)}`, notOut: `Not out yet · in cinemas ${shortDate(theaters, now)}` };
	if (info.status === 'Released') return { words: 'out now', notOut: '' };
	return { words: '', notOut: '' };
}

/** A pill under a show's summary: one season and how much of it was asked for. */
export interface SeasonPill {
	season: number;
	/** "S1: 3 of 7" or "S6: all 16". */
	label: string;
	/** The episodes asked for; empty is the whole season. */
	episodes: number[];
}
export interface AskSummary {
	line: string;
	pills: SeasonPill[];
}

const plural = (n: number, w: string) => `${n} ${w}${n === 1 ? '' : 's'}`;

/** seasonRange says "Season 2", "Seasons 1–3" or "Seasons 1, 3 and 5". */
export function seasonRange(ns: number[]): string {
	const s = [...new Set(ns)].sort((a, b) => a - b);
	if (s.length === 0) return '';
	if (s.length === 1) return `Season ${s[0]}`;
	const run = s.every((n, i) => i === 0 || n === s[i - 1] + 1);
	if (run) return `Seasons ${s[0]}–${s[s.length - 1]}`;
	return `Seasons ${s.slice(0, -1).join(', ')} and ${s[s.length - 1]}`;
}

/**
 * showSummary says what a show ask is for in one line, with season pills:
 * the entire show, whole seasons, a few episodes of one season, or a mix.
 * `episodes` maps a season to the episodes asked for (empty: the whole season).
 */
export function showSummary(
	a: Pick<Ask, 'seasons' | 'future_only' | 'season_words'>,
	seasons: SeasonInfo[],
	episodes?: Record<number, number[]>
): AskSummary {
	const regular = seasons.filter((s) => s.number > 0);
	const count = (n: number) => regular.find((s) => s.number === n)?.episode_count ?? 0;
	if (a.future_only) return { line: 'New episodes from now on', pills: [] };
	const picked = episodes && Object.keys(episodes).length ? Object.keys(episodes).map(Number) : (a.seasons ?? []);
	if (!picked.length) {
		if (!regular.length) return { line: 'The entire show', pills: [] };
		const eps = regular.reduce((n, s) => n + s.episode_count, 0);
		return { line: `${plural(regular.length, 'season')}, ${plural(eps, 'episode')}`, pills: [] };
	}
	const sorted = [...new Set(picked)].sort((x, y) => x - y);
	const pills: SeasonPill[] = sorted.map((n) => {
		const eps = episodes?.[n] ?? [];
		const all = count(n);
		const label = eps.length && (!all || eps.length < all) ? `S${n}: ${eps.length}${all ? ` of ${all}` : ''}` : `S${n}: all${all ? ` ${all}` : ''}`;
		return { season: n, label, episodes: eps.length && (!all || eps.length < all) ? [...eps].sort((x, y) => x - y) : [] };
	});
	const partial = pills.some((p) => p.episodes.length);
	const total = pills.reduce((n, p) => n + (p.episodes.length || count(p.season)), 0);
	if (partial) {
		const line = sorted.length === 1 ? `${plural(total, 'episode')} from season ${sorted[0]}` : `${plural(total, 'episode')} from ${sorted.length} seasons`;
		return { line, pills };
	}
	if (!regular.length) return { line: a.season_words ? a.season_words[0].toUpperCase() + a.season_words.slice(1) : seasonRange(sorted), pills };
	return { line: `${seasonRange(sorted)} · ${sorted.length} of ${plural(regular.length, 'season')}, ${plural(total, 'episode')}`, pills };
}

/** askSummary is an ask's one-line summary: seasons for shows, albums for music; none for movies (the title page has the rest). */
export function askSummary(a: Ask, info: AskInfo | null | undefined): AskSummary {
	if (a.kind === 'artist') {
		const w = a.album_words ?? (a.albums?.length ? a.albums.map((x) => x.name).join(', ') : 'every album');
		return { line: w ? w[0].toUpperCase() + w.slice(1) : '', pills: [] };
	}
	if (a.kind === 'show') return showSummary(a, info?.seasons ?? []);
	return { line: '', pills: [] };
}

/** The season whose episodes say whether part of a show ask is not out yet: the last asked one (or the show's last). */
export function watchSeason(a: Pick<Ask, 'kind' | 'seasons' | 'future_only'>, info: AskInfo | null | undefined, now = new Date()): number {
	if (a.kind !== 'show' || !info) return 0;
	const regular = info.seasons.filter((s) => s.number > 0);
	const asked = a.seasons?.length && !a.future_only ? Math.max(...a.seasons) : (regular[regular.length - 1]?.number ?? 0);
	const s = regular.find((x) => x.number === asked);
	// A season from before last year is out.
	if (!s || (s.year && s.year < now.getFullYear() - 1)) return 0;
	return asked;
}

/** showNotOut says "Not out yet · S6 airs Mar 3" (or "· S4 E7 airs …") when an asked season still has episodes to air. */
export function showNotOut(season: number, eps: EpisodeInfo[] | null | undefined, now = new Date()): string {
	if (!season || !eps?.length) return '';
	const today = now.toISOString().slice(0, 10);
	const next = eps.filter((e) => e.air_day && day(e.air_day) > today).sort((x, y) => (x.air_day ?? '').localeCompare(y.air_day ?? ''))[0];
	if (!next?.air_day) {
		// No date for an episode that has not aired: still waiting, date unknown.
		return eps.every((e) => !e.air_day) ? `Not out yet · S${season} has no air date` : '';
	}
	const first = Math.min(...eps.map((e) => e.number));
	return next.number === first ? `Not out yet · S${season} airs ${shortDate(next.air_day, now)}` : `Not out yet · S${season} E${next.number} airs ${shortDate(next.air_day, now)}`;
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
/** languageName names a base code ("ko" is Korean); unknown codes read as themselves. */
export function languageName(code: string | undefined): string {
	if (!code) return '';
	return audioLanguages.find((l) => l.code === code)?.name ?? code;
}

/** The poster sizes, the same three the Library offers. */
export type InboxSize = 's' | 'm' | 'l';
export const sizeLabels: Record<InboxSize, string> = { s: 'Small', m: 'Medium', l: 'Large' };

type Store = Pick<Storage, 'getItem' | 'setItem'> | undefined;
const key = (what: string, member: number | undefined) => `holyarrs.approvals-${what}.${member ?? 'me'}`;

/** loadSize reads a member's poster size for the Approvals rows (medium when none is kept). */
export function loadSize(member: number | undefined, store: Store = globalThis.localStorage): InboxSize {
	try {
		const v = store?.getItem(key('poster-size', member));
		return v === 's' || v === 'l' ? v : 'm';
	} catch {
		return 'm';
	}
}
export function saveSize(member: number | undefined, s: InboxSize, store: Store = globalThis.localStorage) {
	try {
		store?.setItem(key('poster-size', member), s);
	} catch { /* Private windows may refuse. */ }
}
/** loadPanel reads whether a member keeps the side panel open (open when none is kept). */
export function loadPanel(member: number | undefined, store: Store = globalThis.localStorage): boolean {
	try {
		return store?.getItem(key('panel', member)) !== 'hidden';
	} catch {
		return true;
	}
}
export function savePanel(member: number | undefined, open: boolean, store: Store = globalThis.localStorage) {
	try {
		store?.setItem(key('panel', member), open ? 'shown' : 'hidden');
	} catch { /* Private windows may refuse. */ }
}

export type GroupBy = 'type' | 'person' | 'date';
export const groupByWords: Record<GroupBy, string> = { type: 'Type', person: 'Person', date: 'Date' };
/** loadGroupBy reads how a member groups the Approvals list (by type when none is kept). */
export function loadGroupBy(member: number | undefined, store: Store = globalThis.localStorage): GroupBy {
	try {
		const v = store?.getItem(key('group', member));
		return v === 'person' || v === 'date' ? v : 'type';
	} catch {
		return 'type';
	}
}
export function saveGroupBy(member: number | undefined, g: GroupBy, store: Store = globalThis.localStorage) {
	try {
		store?.setItem(key('group', member), g);
	} catch { /* Private windows may refuse. */ }
}
/** dateGroups splits asks into Today, This week and Older, newest first in each. */
export function dateGroups<T extends { asked_at: string }>(asks: T[], now = new Date()): { id: string; name: string; asks: T[] }[] {
	const day = new Date(now); day.setHours(0, 0, 0, 0);
	const week = day.getTime() - 6 * 864e5;
	const g = [{ id: 'today', name: 'Today', asks: [] as T[] }, { id: 'week', name: 'This week', asks: [] as T[] }, { id: 'older', name: 'Older', asks: [] as T[] }];
	for (const a of [...asks].sort((x, y) => Date.parse(y.asked_at) - Date.parse(x.asked_at))) {
		const t = Date.parse(a.asked_at);
		g[t >= day.getTime() ? 0 : t >= week ? 1 : 2].asks.push(a);
	}
	return g.filter((x) => x.asks.length);
}

/** askers lists the people with asks, by name, for the Asked by filter. */
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

/** askedByOne says whether someone is among the people who asked. */
export function askedByOne(a: Pick<Ask, 'asked_by' | 'people'>, id: number): boolean {
	return a.asked_by.id === id || a.people.some((p) => p.id === id);
}
