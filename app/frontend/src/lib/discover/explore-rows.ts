// Explore rows built from data HolyArrs already has (the Library, the calendar and Requests). Everything that decides what shows, and in which order, is a pure function so it can be tested.

import { type ExploreCard, type ExploreRow, ROW_MAX, titleCard, tmdbArtName, cardTmdb } from '#lib/ui/title-cards.ts';
import type { Ask, CalendarEntry, TitleView } from '../core/types.ts';
import { t, appLocale } from '#lib/core/i18n/i18n.svelte.ts';

/** What two titles can share: genres and people. */
export interface Facets {
	genres: string[];
	people: string[];
}

/** Newest first. Title ids only grow, so the highest id is the latest added. */
export function recentlyAdded(titles: TitleView[], n = ROW_MAX): TitleView[] {
	return [...titles].sort((a, b) => b.id - a.id).slice(0, n);
}

const releaseWord = (): Record<CalendarEntry['kind'], string> => ({ cinema: t('discover.exploreRows.inCinemas'), digital: t('discover.exploreRows.onDigital'), physical: t('discover.exploreRows.onDisc'), episode: '', album: t('discover.exploreRows.newAlbum') });

/** A wide card's second line: S1 · E3 · Name, On digital, or the album. */
export function releaseLine(e: CalendarEntry): string {
	if (e.kind === 'episode') return [`S${e.season ?? 0}`, `E${e.episode ?? 0}`, e.episode_name].filter(Boolean).join(' · ');
	if (e.kind === 'album') return e.album ?? releaseWord().album;
	return releaseWord()[e.kind];
}

/** The day, plus Premiere for S1E1 and New for any other episode 1. */
export function releaseBadges(e: CalendarEntry, now: Date): string[] {
	const when = shortDay(e.kind === 'episode' && !e.timed && e.air_day ? e.air_day : e.date, now);
	const extra = e.kind === 'episode' && e.episode === 1 ? (e.season === 1 ? t('discover.exploreRows.premiere') : t('discover.exploreRows.new')) : '';
	return [when, extra].filter(Boolean);
}

/** Keeps each title's next date, soonest first, skipping what already passed. */
export function comingSoon(entries: CalendarEntry[], titles: TitleView[], now = new Date(), n = ROW_MAX): ExploreCard[] {
	const byId = new Map(titles.map((t) => [t.id, t]));
	const start = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime();
	const seen = new Set<number>();
	const out: ExploreCard[] = [];
	const sorted = [...entries].filter((e) => new Date(e.air_day ?? e.date).getTime() >= start - 86400000).sort((a, b) => a.date.localeCompare(b.date));
	for (const e of sorted) {
		if (seen.has(e.title_id)) continue;
		seen.add(e.title_id);
		const t = byId.get(e.title_id);
		const subtitle = releaseLine(e);
		const card: ExploreCard = t
			? titleCard(t, subtitle, `c${e.title_id}`)
			: { key: `c${e.title_id}`, kind: e.kind === 'episode' ? 'show' : e.kind === 'album' ? 'artist' : 'movie', name: e.name, year: e.year, subtitle, titleId: e.title_id, badges: [] };
		card.subtitle = subtitle;
		card.badges = releaseBadges(e, now);
		card.image = card.image ?? e.poster ?? null;
		// Wide cards: the show or movie backdrop first, then the episode still; none falls back to the name tile.
		card.wide = e.backdrop ?? t?.backdrop ?? e.still ?? null;
		card.status = 'coming_soon';
		out.push(card);
		if (out.length >= n) break;
	}
	return out;
}

/** Shows with aired episodes that have no file, most missing first. */
export function missingEpisodes(titles: TitleView[], n = ROW_MAX): ExploreCard[] {
	return titles
		.filter((t) => (t.show?.missing ?? 0) > 0)
		.sort((a, b) => (b.show!.missing - a.show!.missing) || a.name.localeCompare(b.name))
		.slice(0, n)
		.map((tv) => {
			const m = tv.show!.missing;
			const c = titleCard(tv, m === 1 ? t('discover.exploreRows.missingOne') : t('discover.exploreRows.missingOther', { n: m }), `m${tv.id}`);
			if (c.status === 'in_library') c.status = 'missing';
			return c;
		});
}

export const have = (state: string) => state === 'in_library' || state === 'better_wanted';

/** Counts what two titles share; a shared person counts double a shared genre. */
export function overlap(a: Facets | undefined, b: Facets | undefined): number {
	if (!a || !b) return 0;
	const g = a.genres.filter((x) => b.genres.includes(x)).length;
	const p = a.people.filter((x) => b.people.includes(x)).length;
	return g + 2 * p;
}

/** Ranks a candidate as overlap × recency × household asks. Recency is 1 for the newest title and falls toward 0.5 for the oldest; every person who asked adds 25%. Zero overlap scores zero, so it never shows. */
export function score(o: { overlap: number; ageRank: number; total: number; asks: number }): number {
	const recency = o.total > 1 ? 1 - (0.5 * o.ageRank) / (o.total - 1) : 1;
	return o.overlap * recency * (1 + 0.25 * o.asks);
}

/** The biggest genres in the Library, each with at least `min` titles. */
export function genreRows(titles: TitleView[], facets: Map<number, Facets>, maxRows = 3, min = 4, n = ROW_MAX): ExploreRow[] {
	const by = new Map<string, TitleView[]>();
	for (const t of recentlyAdded(titles, titles.length)) for (const g of facets.get(t.id)?.genres ?? []) by.set(g, [...(by.get(g) ?? []), t]);
	return [...by.entries()]
		.filter(([, ts]) => ts.length >= min)
		.sort(([ga, a], [gb, b]) => b.length - a.length || ga.localeCompare(gb))
		.slice(0, maxRows)
		.map(([g, ts]) => ({ id: `genre-${g.toLowerCase().replace(/[^a-z0-9]+/g, '-')}`, title: t('discover.exploreRows.genreInLibrary', { genre: g }), cards: ts.slice(0, n).map((t) => titleCard(t, undefined, `g${t.id}`)) }));
}

/** Gives each title's first card on the page a view-transition name so its art grows into the title page's hero: title-art-<id> for a Library title, tmdb-art-<kind>-<tmdb> for a movie or show not added yet. */
export function markTransitions(rows: ExploreRow[]): ExploreRow[] {
	const used = new Set<string>();
	return rows.map((r) => ({
		...r,
		cards: r.cards.map((c) => {
			const tmdb = c.kind === 'movie' || c.kind === 'show' ? cardTmdb(c) : '';
			const name = c.titleId !== undefined ? `title-art-${c.titleId}` : tmdb ? tmdbArtName(c.kind as 'movie' | 'show', tmdb) : '';
			if (!name || used.has(name)) return { ...c, vtName: undefined };
			used.add(name);
			return { ...c, vtName: name };
		})
	}));
}

/** Drops empty rows. */
export function keepFull(rows: (ExploreRow | null | undefined)[]): ExploreRow[] {
	return rows.filter((r): r is ExploreRow => !!r && (r.cards.length > 0 || !!r.keep));
}

function shortDay(iso: string, now: Date): string {
	const d = new Date(iso);
	if (Number.isNaN(d.getTime())) return '';
	const day = (x: Date) => Math.floor(Date.UTC(x.getFullYear(), x.getMonth(), x.getDate()) / 86400000);
	const diff = day(d) - day(now);
	if (diff === 0) return t('discover.exploreRows.today');
	if (diff === 1) return t('discover.exploreRows.tomorrow');
	return d.toLocaleDateString(appLocale(), diff < 7 && diff > 0 ? { weekday: 'long' } : { day: 'numeric', month: 'short' });
}

/** Groups asks for the same thing (the same title, or the same add id). */
function askKey(a: Pick<Ask, 'title_id' | 'ref' | 'kind' | 'tmdb_id' | 'name'>): string {
	if (a.title_id) return `t${a.title_id}`;
	if (a.ref) return a.ref;
	if (a.tmdb_id) return `tmdb:${a.tmdb_id}`;
	return `${a.kind}:${a.name}`;
}

function asksOf(t: Pick<TitleView, 'asks'>): number {
	return (t.asks ?? []).reduce((n, a) => n + Math.max(1, a.people?.length ?? 0), 0);
}
