// Explore: rows of titles built from data HolyArrs already has (the Library,
// the calendar and the Wishlist), plus rows from a later discover endpoint.
// Everything that decides what shows, and in which order, lives here as pure
// functions so it can be tested and explained.
import { api, ApiError, type Client } from './api.ts';
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
}

export interface ExploreRow {
	id: string;
	title: string;
	subtitle?: string;
	href?: string;
	cards: ExploreCard[];
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

/** markTransitions gives each title's first card on the page the view-transition name title-art-<id>. */
export function markTransitions(rows: ExploreRow[]): ExploreRow[] {
	const used = new Set<number>();
	return rows.map((r) => ({
		...r,
		cards: r.cards.map((c) => {
			if (c.titleId === undefined || used.has(c.titleId)) return { ...c, vtName: undefined };
			used.add(c.titleId);
			return { ...c, vtName: `title-art-${c.titleId}` };
		})
	}));
}

/** keepFull drops empty rows. */
export function keepFull(rows: (ExploreRow | null | undefined)[]): ExploreRow[] {
	return rows.filter((r): r is ExploreRow => !!r && r.cards.length > 0);
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

export type ProviderRowName = 'trending' | 'popular' | 'upcoming' | 'similar' | 'genre';
export interface ProviderRowQuery {
	row: ProviderRowName;
	/** similar: the Library title id. */
	id?: number;
	/** genre: the genre name. */
	g?: string;
	kind?: 'movie' | 'show';
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
		mine: !!d.ask?.mine,
		ask: d.ask && d.ask.state !== 'turned_down' ? d.ask : undefined
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
	popular: 'Popular right now',
	upcoming: 'Coming to cinemas'
};
