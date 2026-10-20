// Explore: rows of titles built from data HolyArrs already has (the Library,
// the calendar and the Wishlist), plus rows from a later discover endpoint.
// Everything that decides what shows, and in which order, lives here as pure
// functions so it can be tested and explained.
import { pickFrom, type CardArtPick } from './card-art.ts';
import { api, ApiError, type Client } from './api.ts';
import type { Fresh } from './fresh.ts';
import type { StatusKind } from './ui/StatusPill.svelte';
import { audienceStatus, factsOf, titleStatus, type Audience, type TitleStatus } from './title-status.ts';
import type { Ask, AskBrief, AskState, CalendarEntry, TitleView } from './types.ts';

export type CardKind = 'movie' | 'show' | 'artist';
export type CardStatus = StatusKind;

/** ExploreCard is one poster in a row, whatever it came from. */
export interface ExploreCard {
	/** Unique within its row. */
	key: string;
	kind: CardKind;
	name: string;
	year?: number;
	image?: string | null;
	subtitle?: string;
	/** Set when the title is in the Library. */
	titleId?: number;
	/** How it is added or asked for, such as tmdb:693134 (not in the Library). */
	ref?: string;
	status?: CardStatus;
	statusLabel?: string;
	badges: string[];
	/** Whether the signed-in person already asked for it. */
	mine?: boolean;
	/** view-transition-name; only the first card of a title on a page gets one. */
	vtName?: string;
	/** The open ask for it, so asking says it joins that ask. */
	ask?: AskBrief;
	/** 16:9 picture for wide cards: backdrop, else episode still. */
	wide?: string | null;
	/** An ask for it was turned down (shown to staff only). */
	declined?: boolean;
	/** The server's one freshness strip. */
	fresh?: Fresh | null;
	/** The wide card's picture, picked on the server (card system). */
	art?: CardArtPick;
}

export interface ExploreRow {
	id: string;
	title: string;
	subtitle?: string;
	href?: string;
	cards: ExploreCard[];
	/** Shown even with no cards (a row with its own picker). */
	keep?: boolean;
	/** Server rows: the row type, and wide cards (Coming soon). */
	type?: string;
	wide?: boolean;
	/** Server rows: where the title came from (fixed, plain or a template layer), for measuring. */
	titleLayer?: string;
}

/** Facets are what two titles can share: genres and people. */
export interface Facets {
	genres: string[];
	people: string[];
}

export const ROW_MAX = 20;

const kindWord: Record<CardKind, string> = { movie: 'Movie', show: 'Show', artist: 'Artist' };

function kindOf(k: string): CardKind {
	return k === 'show' || k === 'artist' ? k : 'movie';
}

/** quality is the best file's resolution in plain marks: 4K, 1080p, 720p. */
export function quality(t: Pick<TitleView, 'slots'>): string | undefined {
	let best = 0;
	for (const s of t.slots ?? []) {
		const r = Number((s.file?.Quality as { resolution?: unknown } | null | undefined)?.resolution ?? 0);
		if (r > best) best = r;
	}
	if (best >= 2000) return '4K';
	if (best >= 1000) return '1080p';
	if (best >= 700) return '720p';
	return best > 0 ? `${best}p` : undefined;
}

/** titleCard turns a Library title into a card. */
export function titleCard(t: TitleView, subtitle?: string, key = `t${t.id}`): ExploreCard {
	const s = titleStatus(factsOf(t), 'staff');
	const q = quality(t);
	return {
		key,
		kind: kindOf(t.kind),
		name: t.name,
		year: t.year,
		image: t.artwork?.[0] ?? null,
		subtitle: subtitle ?? (t.year ? String(t.year) : kindWord[kindOf(t.kind)]),
		titleId: t.id,
		status: s.kind,
		badges: q ? [q] : []
	};
}

/** recentlyAdded: newest first. Title ids only grow, so the highest id is the latest added. */
export function recentlyAdded(titles: TitleView[], n = ROW_MAX): TitleView[] {
	return [...titles].sort((a, b) => b.id - a.id).slice(0, n);
}

/** orderRecent puts cards with artwork first, and within each group takes turns between kinds (order kept per kind). */
export function orderRecent<T extends { kind: CardKind; image?: string | null }>(items: T[]): T[] {
	const turns = (list: T[]): T[] => {
		const byKind = new Map<CardKind, T[]>();
		for (const x of list) byKind.set(x.kind, [...(byKind.get(x.kind) ?? []), x]);
		const queues = [...byKind.values()];
		const out: T[] = [];
		while (out.length < list.length) for (const q of queues) if (q.length) out.push(q.shift()!);
		return out;
	};
	return [...turns(items.filter((x) => !!x.image)), ...turns(items.filter((x) => !x.image))];
}

function shortDay(iso: string, now: Date): string {
	const d = new Date(iso);
	if (Number.isNaN(d.getTime())) return '';
	const day = (x: Date) => Math.floor(Date.UTC(x.getFullYear(), x.getMonth(), x.getDate()) / 86400000);
	const diff = day(d) - day(now);
	if (diff === 0) return 'Today';
	if (diff === 1) return 'Tomorrow';
	return d.toLocaleDateString(undefined, diff < 7 && diff > 0 ? { weekday: 'long' } : { day: 'numeric', month: 'short' });
}

const releaseWord: Record<CalendarEntry['kind'], string> = { cinema: 'In cinemas', digital: 'On digital', physical: 'On disc', episode: '', album: 'New album' };

/** releaseLine is a wide card's second line: S1 · E3 · Name, On digital, or the album. */
export function releaseLine(e: CalendarEntry): string {
	if (e.kind === 'episode') return [`S${e.season ?? 0}`, `E${e.episode ?? 0}`, e.episode_name].filter(Boolean).join(' · ');
	if (e.kind === 'album') return e.album ?? releaseWord.album;
	return releaseWord[e.kind];
}

/** releaseBadges: the day, plus Premiere for S1E1 and New for any other episode 1. */
export function releaseBadges(e: CalendarEntry, now: Date): string[] {
	const when = shortDay(e.kind === 'episode' && !e.timed && e.air_day ? e.air_day : e.date, now);
	const extra = e.kind === 'episode' && e.episode === 1 ? (e.season === 1 ? 'Premiere' : 'New') : '';
	return [when, extra].filter(Boolean);
}

/** comingSoon keeps each title's next date, soonest first, skipping what already passed. */
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

/** missingEpisodes lists shows with aired episodes that have no file, most missing first. */
export function missingEpisodes(titles: TitleView[], n = ROW_MAX): ExploreCard[] {
	return titles
		.filter((t) => (t.show?.missing ?? 0) > 0)
		.sort((a, b) => (b.show!.missing - a.show!.missing) || a.name.localeCompare(b.name))
		.slice(0, n)
		.map((t) => {
			const m = t.show!.missing;
			const c = titleCard(t, `${m} ${m === 1 ? 'episode' : 'episodes'} missing`, `m${t.id}`);
			if (c.status === 'in_library') c.status = 'missing';
			return c;
		});
}

/** kindShelf: the newest titles of one kind, for the Movies, TV shows and Music rows. */
export function kindShelf(titles: TitleView[], kind: CardKind, n = ROW_MAX): ExploreCard[] {
	return recentlyAdded(titles.filter((t) => kindOf(t.kind) === kind), n).map((t) => titleCard(t, undefined, `${kind[0]}${t.id}`));
}

const have = (state: string) => state === 'in_library' || state === 'better_wanted';

/** readyToWatch: movies and shows with every copy in the Library and no episode missing. */
export function readyToWatch(titles: TitleView[], n = ROW_MAX): ExploreCard[] {
	return recentlyAdded(
		titles.filter((t) => kindOf(t.kind) !== 'artist' && (t.slots ?? []).length > 0 && t.slots.every((s) => have(s.state)) && !(t.show?.missing ?? 0)),
		n
	).map((t) => titleCard(t, undefined, `r${t.id}`));
}

/** stillLooking: movies and artists still looking for a copy (shows have Missing episodes). */
export function stillLooking(titles: TitleView[], n = ROW_MAX): ExploreCard[] {
	return recentlyAdded(titles.filter((t) => kindOf(t.kind) !== 'show' && (t.slots ?? []).some((s) => s.state === 'wanted')), n).map((t) => {
		const c = titleCard(t, undefined, `w${t.id}`);
		if (c.status === 'in_library') c.status = 'missing';
		return c;
	});
}

/** askKey groups asks for the same thing (the same title, or the same add id). */
function askKey(a: Pick<Ask, 'title_id' | 'ref' | 'kind' | 'tmdb_id' | 'name'>): string {
	if (a.title_id) return `t${a.title_id}`;
	if (a.ref) return a.ref;
	if (a.tmdb_id) return `tmdb:${a.tmdb_id}`;
	return `${a.kind}:${a.name}`;
}

/**
 * popularInHousehold: what the most people asked for. Score is the number of
 * people on the ask; ties go to the latest ask. Turned-down asks do not count.
 */
export function popularInHousehold(asks: Ask[], titles: TitleView[], n = ROW_MAX): ExploreCard[] {
	const byId = new Map(titles.map((t) => [t.id, t]));
	const groups = new Map<string, { ask: Ask; people: Set<number>; last: string; mine: boolean }>();
	for (const a of asks) {
		if (a.state === 'turned_down') continue;
		const k = askKey(a);
		const g = groups.get(k) ?? { ask: a, people: new Set<number>(), last: a.asked_at, mine: false };
		for (const p of a.people?.length ? a.people : [a.asked_by]) if (p) g.people.add(p.id);
		if (a.asked_at > g.last) g.last = a.asked_at;
		g.mine ||= !!a.mine;
		groups.set(k, g);
	}
	return [...groups.entries()]
		.sort(([, x], [, y]) => y.people.size - x.people.size || y.last.localeCompare(x.last))
		.slice(0, n)
		.map(([k, g]) => {
			const who = `${g.people.size} ${g.people.size === 1 ? 'person' : 'people'} asked`;
			const t = g.ask.title_id ? byId.get(g.ask.title_id) : undefined;
			if (t) return { ...titleCard(t, who, `p${k}`), mine: g.mine };
			const done = g.ask.state === 'available';
			return {
				key: `p${k}`,
				kind: kindOf(g.ask.kind),
				name: g.ask.name,
				year: g.ask.year,
				image: g.ask.poster ?? null,
				subtitle: who,
				titleId: g.ask.title_id,
				ref: g.ask.ref || (g.ask.tmdb_id ? `tmdb:${g.ask.tmdb_id}` : undefined),
				status: titleStatus({ ask: g.ask.state }, 'staff').kind,
				badges: [],
				mine: g.mine,
				ask: { id: g.ask.id, state: g.ask.state, state_label: g.ask.state_label, mine: g.mine, people: g.people.size }
			} satisfies ExploreCard;
		});
}

/** overlap counts what two titles share; a shared person counts double a shared genre. */
export function overlap(a: Facets | undefined, b: Facets | undefined): number {
	if (!a || !b) return 0;
	const g = a.genres.filter((x) => b.genres.includes(x)).length;
	const p = a.people.filter((x) => b.people.includes(x)).length;
	return g + 2 * p;
}

/**
 * score ranks a candidate: overlap × recency × household asks. recency is
 * 1 for the newest title and falls towards 0.5 for the oldest; every person
 * who asked adds 25%. Zero overlap scores zero, so it never shows.
 */
export function score(o: { overlap: number; ageRank: number; total: number; asks: number }): number {
	const recency = o.total > 1 ? 1 - (0.5 * o.ageRank) / (o.total - 1) : 1;
	return o.overlap * recency * (1 + 0.25 * o.asks);
}

function asksOf(t: Pick<TitleView, 'asks'>): number {
	return (t.asks ?? []).reduce((n, a) => n + Math.max(1, a.people?.length ?? 0), 0);
}

/**
 * becauseYouHave picks the newest title that has at least `min` related
 * titles and returns it with those titles, best first.
 */
export function becauseYouHave(titles: TitleView[], facets: Map<number, Facets>, min = 3, n = ROW_MAX): { seed: TitleView; cards: ExploreCard[] } | null {
	const recent = recentlyAdded(titles, titles.length);
	const rank = new Map(recent.map((t, i) => [t.id, i]));
	for (const seed of recent) {
		const f = facets.get(seed.id);
		if (!f) continue;
		const scored = recent
			.filter((t) => t.id !== seed.id && t.kind === seed.kind)
			.map((t) => ({ t, s: score({ overlap: overlap(f, facets.get(t.id)), ageRank: rank.get(t.id) ?? 0, total: recent.length, asks: asksOf(t) }) }))
			.filter((x) => x.s > 0)
			.sort((a, b) => b.s - a.s || a.t.name.localeCompare(b.t.name));
		if (scored.length >= min) return { seed, cards: scored.slice(0, n).map((x) => titleCard(x.t, undefined, `b${x.t.id}`)) };
	}
	return null;
}

/** genreRows: the biggest genres in the Library, each with at least `min` titles. */
export function genreRows(titles: TitleView[], facets: Map<number, Facets>, maxRows = 3, min = 4, n = ROW_MAX): ExploreRow[] {
	const by = new Map<string, TitleView[]>();
	for (const t of recentlyAdded(titles, titles.length)) for (const g of facets.get(t.id)?.genres ?? []) by.set(g, [...(by.get(g) ?? []), t]);
	return [...by.entries()]
		.filter(([, ts]) => ts.length >= min)
		.sort(([ga, a], [gb, b]) => b.length - a.length || ga.localeCompare(gb))
		.slice(0, maxRows)
		.map(([g, ts]) => ({ id: `genre-${g.toLowerCase().replace(/[^a-z0-9]+/g, '-')}`, title: `${g} in your library`, cards: ts.slice(0, n).map((t) => titleCard(t, undefined, `g${t.id}`)) }));
}

/** fromPerson: the person in the most Library titles (at least `min`), with those titles. */
export function fromPerson(titles: TitleView[], facets: Map<number, Facets>, min = 2, n = ROW_MAX): { person: string; cards: ExploreCard[] } | null {
	const count = new Map<string, TitleView[]>();
	for (const t of recentlyAdded(titles, titles.length)) for (const p of new Set(facets.get(t.id)?.people ?? [])) count.set(p, [...(count.get(p) ?? []), t]);
	const best = [...count.entries()].filter(([, ts]) => ts.length >= min).sort(([pa, a], [pb, b]) => b.length - a.length || pa.localeCompare(pb))[0];
	return best ? { person: best[0], cards: best[1].slice(0, n).map((t) => titleCard(t, undefined, `f${t.id}`)) } : null;
}

/** genresOf reads genres when the API sends them (it does not yet; see backend-needs.md). */
export function genresOf(t: TitleView): string[] {
	const g = (t as TitleView & { genres?: unknown }).genres;
	return Array.isArray(g) ? g.filter((x): x is string => typeof x === 'string') : [];
}

/**
 * markTransitions gives each title's first card on the page a view-transition
 * name, so its art grows into the title page's hero: title-art-<id> for a
 * Library title, tmdb-art-<kind>-<tmdb> for a movie or show not added yet.
 */
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

/** tmdbArtName is the view-transition name of a not-added movie or show's art (card and hero). */
export const tmdbArtName = (kind: 'movie' | 'show', tmdb: string) => `tmdb-art-${kind}-${tmdb}`;

/** keepFull drops empty rows. */
export function keepFull(rows: (ExploreRow | null | undefined)[]): ExploreRow[] {
	return rows.filter((r): r is ExploreRow => !!r && (r.cards.length > 0 || !!r.keep));
}

/**
 * cardPill is the StatusPill a card shows, in the shared status words
 * (title-status.ts) for this audience; a label set on the card wins.
 */
export function cardPill(c: ExploreCard, who: Audience): TitleStatus | null {
	if (!c.status) return null;
	if (c.statusLabel) return { kind: c.status, label: c.statusLabel };
	return audienceStatus(c.status, who, { tracked: c.titleId !== undefined, titleKind: c.kind });
}

/** cardAction says what the round + does for this person on this card. */
export function cardAction(c: ExploreCard, can: { manage: boolean; ask: boolean }): 'add' | 'ask' | null {
	if (c.titleId !== undefined || c.status === 'in_library') return null;
	if (can.manage) return 'add';
	if (can.ask && !c.mine && c.ref) return 'ask';
	return null;
}

/** cardTmdb is the TMDB id of a movie or show card not in the Library, or ''. */
export function cardTmdb(c: Pick<ExploreCard, 'kind' | 'ref'>): string {
	if ((c.kind !== 'movie' && c.kind !== 'show') || !c.ref?.startsWith('tmdb:')) return '';
	return c.ref.slice(5);
}

// ---- Rows from the discover endpoint (not built yet; see backend-needs.md) ----

export type ProviderRowName = 'trending' | 'best' | 'upcoming' | 'similar' | 'genre' | 'services';
export interface ProviderRowQuery {
	row: ProviderRowName;
	/** similar: the Library title id. */
	id?: number;
	/** genre: the genre name. */
	g?: string;
	kind?: 'movie' | 'show';
	/** services: streaming service ids. */
	services?: number[];
	/** services: the two-letter country. */
	region?: string;
}
export interface ProviderItem {
	tmdb_id: string;
	kind: 'movie' | 'show';
	name: string;
	year?: number;
	poster?: string | null;
	backdrop?: string | null;
	overview?: string;
	genres?: string[];
	in_library: boolean;
	title_id?: number;
	ask?: { id: number; state: Ask['state']; state_label: string; mine: boolean; people: number } | null;
	fresh?: Fresh | null;
	/** Explore: the YouTube trailer, when the server already knows it. */
	trailer_key?: string;
	/** The wide card's picture, picked on the server (card system). */
	card_art?: string;
	card_logo?: string;
	art_has_title?: boolean;
}
export interface ProviderRowAnswer {
	row: ProviderRowName;
	title?: string;
	items: ProviderItem[];
}

export function discoverPath(q: ProviderRowQuery): string {
	const p = new URLSearchParams({ row: q.row });
	if (q.id !== undefined) p.set('id', String(q.id));
	if (q.g) p.set('g', q.g);
	if (q.kind) p.set('kind', q.kind);
	if (q.services?.length) p.set('services', q.services.join('|'));
	if (q.region) p.set('region', q.region);
	return `/discover?${p.toString()}`;
}

/** fetchProviderRow asks the server for a discover row; null when the server has none yet (404/501). */
export async function fetchProviderRow(q: ProviderRowQuery, client: Pick<Client, 'get'> = api, signal?: AbortSignal): Promise<ProviderRowAnswer | null> {
	try {
		return await client.get<ProviderRowAnswer>(discoverPath(q), signal);
	} catch (e) {
		if (e instanceof ApiError && (e.status === 404 || e.status === 501)) return null;
		throw e;
	}
}

/** discoverCard turns a discover item into a card. */
export function discoverCard(d: ProviderItem, row: string): ExploreCard {
	return {
		key: `${row}-${d.kind}-${d.tmdb_id}`,
		kind: d.kind,
		name: d.name,
		year: d.year,
		image: d.poster ?? null,
		subtitle: d.year ? String(d.year) : undefined,
		titleId: d.in_library ? d.title_id : undefined,
		ref: `tmdb:${d.tmdb_id}`,
		status: d.in_library ? 'in_library' : d.ask && d.ask.state !== 'turned_down' ? titleStatus({ ask: d.ask.state }, 'staff').kind : undefined,
		badges: [],
		fresh: d.fresh ?? null,
		art: pickFrom(d),
		mine: !!d.ask?.mine,
		ask: d.ask && d.ask.state !== 'turned_down' ? d.ask : undefined,
		...(d.ask?.state === 'turned_down' ? { declined: true } : {})
	};
}

/**
 * liveCard is a card after something happened to it on this page: asked
 * (its pill follows the ask state the server returned, so an auto-approved
 * ask reads Approved) or refused because it is in the Library already.
 */
export function liveCard(c: ExploreCard, here: { ask?: AskState; titleId?: number } | undefined): ExploreCard {
	if (here?.titleId !== undefined) return { ...c, titleId: here.titleId, status: 'in_library', statusLabel: undefined };
	if (here?.ask) return { ...c, mine: true, status: titleStatus({ ask: here.ask }, 'staff').kind, statusLabel: undefined };
	return c;
}

export const discoverTitles: Record<Exclude<ProviderRowName, 'similar' | 'genre'>, string> = {
	trending: 'Trending this week',
	best: 'Best new releases',
	services: 'On your services',
	upcoming: 'Coming to cinemas'
};

// ---- "On your services": the streaming services a person has ----

/** WatchService is one streaming service from GET /watch-services. */
export interface WatchService {
	id: number;
	name: string;
	logo?: string;
	priority: number;
}

/** SERVICES_SHOWN: how many services the picker offers (the region's best known first). */
export const SERVICES_SHOWN = 6;

/** serviceRegion: the person's country (as for Top 10), else US. */
export function serviceRegion(r: string): string {
	return /^[A-Z]{2}$/.test(r) ? r : 'US';
}

/** pickerServices: the services the picker shows, the region's top few plus any picked that fell out of it. */
export function pickerServices(all: WatchService[], picked: number[], n = SERVICES_SHOWN): WatchService[] {
	const top = [...all].sort((a, b) => a.priority - b.priority).slice(0, n);
	const extra = all.filter((s) => picked.includes(s.id) && !top.some((t) => t.id === s.id));
	return [...top, ...extra];
}

/** toggleService adds or removes one service id, keeping the order picked. */
export function toggleService(picked: number[], id: number): number[] {
	return picked.includes(id) ? picked.filter((x) => x !== id) : [...picked, id];
}

/** Saved choice: { region: [ids] }, null when never picked (so the first-time prompt shows). */
export const SERVICES_KEY = 'holyarrs.explore.services';
export function readServices(region: string, raw: string | null): number[] | null {
	try {
		const v = JSON.parse(raw ?? 'null')?.[region];
		return Array.isArray(v) ? v.filter((x): x is number => Number.isInteger(x) && x > 0) : null;
	} catch {
		return null;
	}
}
export function writeServices(region: string, ids: number[], raw: string | null): string {
	let all: Record<string, number[]> = {};
	try {
		const v = JSON.parse(raw ?? 'null');
		if (v && typeof v === 'object' && !Array.isArray(v)) all = v;
	} catch {
		/* start over */
	}
	return JSON.stringify({ ...all, [region]: ids });
}

// ---- The type switch (All · Movies · Shows · Music) ----

export type ExploreType = 'all' | 'movies' | 'shows' | 'music';
export const exploreTypes: ExploreType[] = ['all', 'movies', 'shows', 'music'];
const typeKind: Record<Exclude<ExploreType, 'all'>, CardKind> = { movies: 'movie', shows: 'show', music: 'artist' };
/** The shelves that hold one kind only, and the type they belong to. */
const kindRows: Record<string, Exclude<ExploreType, 'all'>> = { movies: 'movies', shows: 'shows', music: 'music' };

/** parseType reads ?type=; anything unknown (or music while it is off) is All. */
export function parseType(v: string | null | undefined, musicOn = true): ExploreType {
	if (v === 'movies' || v === 'shows' || (v === 'music' && musicOn)) return v;
	return 'all';
}

/** typeKindOf is the card kind a type keeps, or null for All. */
export function typeKindOf(t: ExploreType): CardKind | null {
	return t === 'all' ? null : typeKind[t];
}

/**
 * filterRows keeps the cards of one type in every row, drops the kind shelves
 * of other types, and hides rows left empty. All keeps everything.
 */
export function filterRows(rows: ExploreRow[], t: ExploreType): ExploreRow[] {
	const kind = typeKindOf(t);
	if (!kind) return rows;
	return keepFull(rows.filter((r) => !kindRows[r.id] || kindRows[r.id] === t).map((r) => ({ ...r, cards: r.cards.filter((c) => c.kind === kind) })));
}

/** typeHref carries the type to a "See all" target: kind= for the Library, type= elsewhere. */
export function typeHref(href: string | undefined, t: ExploreType): string | undefined {
	const kind = typeKindOf(t);
	if (!href || !kind) return href;
	const [path, query = ''] = href.split('?');
	const p = new URLSearchParams(query);
	if (/\/library\/?$/.test(path)) {
		if (p.has('kind') && p.get('kind') !== kind) return href;
		p.set('kind', kind);
	} else p.set('type', t);
	return `${path}?${p.toString()}`;
}

// ---- Owned marks (Overseerr-style: type pill top-left, status icon top-right) ----

/** OwnedState is the round status icon on a poster; none means not in the Library and not asked. */
export type OwnedState = 'available' | 'partial' | 'downloading' | 'requested' | 'declined';
export interface OwnedMark {
	state: OwnedState;
	/** aria-label and tooltip. */
	label: string;
}

/** ownedKey is how a title is found again: kind plus its TMDB id (MusicBrainz id for artists). */
function ownedKeys(kind: CardKind, ids: Record<string, string> | null | undefined): string[] {
	const out: string[] = [];
	if (ids?.tmdb) out.push(`${kind}:tmdb:${ids.tmdb}`);
	if (ids?.musicbrainz) out.push(`${kind}:mbid:${ids.musicbrainz}`);
	return out;
}

/** OwnedIndex finds Library titles by id or by their TMDB / MusicBrainz id. */
export interface OwnedIndex {
	byId: Map<number, TitleView>;
	byRef: Map<string, TitleView>;
}

export function ownedIndex(titles: TitleView[]): OwnedIndex {
	const byId = new Map<number, TitleView>();
	const byRef = new Map<string, TitleView>();
	for (const t of titles) {
		byId.set(t.id, t);
		const mbid = t.artist?.mbid;
		for (const k of ownedKeys(kindOf(t.kind), { ...(t.ids ?? {}), ...(mbid ? { musicbrainz: mbid } : {}) })) byRef.set(k, t);
	}
	return { byId, byRef };
}

/** ownedTitle is the Library title behind a card, if any. */
export function ownedTitle(c: Pick<ExploreCard, 'kind' | 'ref' | 'titleId'>, idx: OwnedIndex): TitleView | undefined {
	if (c.titleId !== undefined) return idx.byId.get(c.titleId);
	if (!c.ref) return undefined;
	const [scheme, id] = [c.ref.slice(0, c.ref.indexOf(':')), c.ref.slice(c.ref.indexOf(':') + 1)];
	if (!id || (scheme !== 'tmdb' && scheme !== 'mbid')) return undefined;
	return idx.byRef.get(`${c.kind}:${scheme}:${id}`);
}

const fullStates = new Set(['in_library', 'better_wanted', 'no_hardlink']);

const ownedWords: Record<OwnedState, string> = {
	available: 'In your library',
	partial: 'Partly available',
	downloading: 'Downloading',
	requested: 'Requested',
	declined: 'Declined'
};

/**
 * ownedMark says which round icon a card shows. A Library title is checked
 * first (downloading, then all or part of its files); otherwise the card's
 * own ask. Declined shows only to people who manage or approve (staff).
 */
export function ownedMark(c: ExploreCard, t: TitleView | undefined, who: Audience): OwnedMark | null {
	const mark = (state: OwnedState, label = ownedWords[state]): OwnedMark => ({ state, label });
	if (t) {
		const f = factsOf(t);
		if (f.downloading) return mark('downloading', f.percent !== undefined ? `Downloading ${f.percent}%` : undefined);
		if (f.inLibrary) {
			const slots = t.slots ?? [];
			const part = (t.show?.missing ?? 0) > 0 || (t.artist?.missing ?? 0) > 0 || slots.some((s) => !fullStates.has(s.state) && !s.file && s.state !== 'not_out_yet');
			return mark(part ? 'partial' : 'available');
		}
		if (f.ask === 'turned_down') return who === 'staff' ? mark('declined') : null;
		return mark('requested', f.ask === 'approved' ? 'Approved, waiting for a copy' : f.ask ? 'Requested' : 'Waiting for a copy');
	}
	switch (c.status) {
		case 'in_library':
			return mark('available');
		case 'downloading':
			return mark('downloading');
		case 'missing':
			return c.titleId !== undefined ? mark('requested', 'Waiting for a copy') : null;
		case 'requested':
			return mark('requested');
		case 'approved':
			return mark('requested', 'Approved, waiting for a copy');
		case 'turned_down':
			return who === 'staff' && c.ask?.state === 'turned_down' ? mark('declined') : null;
	}
	return c.declined && who === 'staff' ? mark('declined') : null;
}

/** matchOwned ties a card to its Library title (found by TMDB or MusicBrainz id), so owned cards open the title page and lose the +. */
export function matchOwned(c: ExploreCard, idx: OwnedIndex): ExploreCard {
	const t = ownedTitle(c, idx);
	if (!t || c.titleId === t.id) return c;
	return { ...c, titleId: t.id, status: titleStatus(factsOf(t), 'staff').kind, statusLabel: undefined };
}

// ---- The server-built page: head in one request, tail by pages ----

export interface ServerRow {
	id: string;
	type: string;
	title: string;
	/** A small line under the title, chosen by the server. */
	subtitle?: string;
	title_layer?: string;
	wide?: boolean;
	items: ProviderItem[];
}
export interface HeadAnswer {
	type: ExploreType;
	building: boolean;
	seeds: number;
	tail_cap: number;
	tail_rows: number;
	top10: (import('./top10.ts').Top10Answer & { kind?: 'movie' | 'show' }) | null;
	/** Up to 6 titles not in the Library (from Top 10 and Trending) for the hero. */
	hero?: ProviderItem[];
	head: ServerRow[];
}
export interface TailAnswer {
	page: number;
	more: boolean;
	rows: ServerRow[];
}
export interface HeadQuery {
	type: ExploreType;
	services?: number[];
	region?: string;
	top10Kind?: 'movie' | 'show';
	top10Region?: string;
	/** The viewer's time zone (IANA), so row titles fit their time of day. */
	tz?: string;
}

function headParams(q: HeadQuery): URLSearchParams {
	const p = new URLSearchParams({ type: q.type === 'music' ? 'all' : q.type });
	if (q.top10Kind) p.set('top10_kind', q.top10Kind);
	if (q.top10Region) p.set('top10_region', q.top10Region);
	if (q.tz) p.set('tz', q.tz);
	return p;
}

/** headPath: services are sent so the server builds On your services for them (empty = none picked). */
export function headPath(q: HeadQuery): string {
	const p = headParams(q);
	if (q.services && q.region) {
		p.set('services', q.services.join('|'));
		p.set('region', q.region);
	}
	return `/explore/head?${p.toString()}`;
}

export function tailPath(q: HeadQuery, page: number): string {
	const p = headParams(q);
	p.set('page', String(page));
	return `/explore/tail?${p.toString()}`;
}

/** Badges state facts only (revision 4): no countdowns, no "leaving soon". */
export const FACTUAL_BADGES = /^(New season|New episodes|Coming .+|In library|Asked|Today|Tomorrow|Premiere|New|4K|1080p|720p|\d+p)$/;
export function factualBadges(badges: string[]): string[] {
	return badges.filter((b) => FACTUAL_BADGES.test(b));
}

/** serverRow turns a server row into an Explore row; Coming soon keeps its wide pictures. */
export function serverRow(r: ServerRow): ExploreRow & { wide: boolean; type: string } {
	return {
		id: r.id,
		type: r.type,
		title: r.title,
		...(r.subtitle ? { subtitle: r.subtitle } : {}),
		...(r.title_layer ? { titleLayer: r.title_layer } : {}),
		wide: !!r.wide,
		cards: r.items.map((d) => {
			// Every movie and show row draws wide title cards now: keep the backdrop on each card.
			const c = { ...discoverCard(d, r.id), wide: d.backdrop ?? null };
			return { ...c, badges: factualBadges(c.badges) };
		}),
		keep: r.type === 'services' || r.type === 'top10'
	};
}
