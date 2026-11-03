// Details for a title that is not (yet) in the Library: /title/movie/<tmdb>
// and /title/show/<tmdb>. GET /metadata/{kind}/{tmdb} is tried first (a
// backend branch adds it); on 404 the page falls back to a lookup by id,
// which also says whether the Library has it and who asked.
import { ApiError } from './api.ts';
import { shownScore } from './rating.ts';
import type { AskBrief, LookupMovie, LookupShow } from './types.ts';

export type DetailKind = 'movie' | 'show';

export interface DetailPerson {
	name: string;
	role?: string;
	image?: string | null;
	id?: string;
}

export interface DetailCard {
	kind: DetailKind;
	tmdb: string;
	name: string;
	year?: number;
	poster?: string | null;
}

export interface TitleDetails {
	kind: DetailKind;
	tmdb: string;
	name: string;
	year?: number;
	overview?: string;
	poster?: string | null;
	backdrop?: string | null;
	genres: string[];
	runtime?: number;
	rating?: string;
	network?: string;
	cast: DetailPerson[];
	similar: DetailCard[];
	titleId?: number;
	neverAdd?: boolean;
	ask?: AskBrief;
	/** A movie's open 4K ask, and whether the Library copy has 4K already. */
	ask4k?: AskBrief;
	has4k?: boolean;
	/** The lookup answer, for the Add and Ask dialogs. */
	movie?: LookupMovie;
	show?: LookupShow;
}

export function isDetailKind(k: string): k is DetailKind {
	return k === 'movie' || k === 'show';
}

export function detailPath(kind: DetailKind, tmdb: string): string {
	return `/title/${kind}/${encodeURIComponent(tmdb)}`;
}

// What a search popover or Explore already knew, for a first paint.
const seen = new Map<string, Partial<TitleDetails>>();
export function remember(kind: DetailKind, tmdb: string, d: Partial<TitleDetails>): void {
	seen.set(`${kind}:${tmdb}`, d);
}
export function remembered(kind: DetailKind, tmdb: string): Partial<TitleDetails> | undefined {
	return seen.get(`${kind}:${tmdb}`);
}

type Raw = Record<string, unknown>;
const str = (v: unknown) => (typeof v === 'string' && v.trim() ? v.trim() : undefined);
const num = (v: unknown) => (typeof v === 'number' && Number.isFinite(v) && v > 0 ? v : undefined);
const list = (v: unknown): Raw[] => (Array.isArray(v) ? (v.filter((x) => x && typeof x === 'object') as Raw[]) : []);

/** fromMetadata reads a /metadata/{kind}/{tmdb} answer, tolerating missing fields. */
export function fromMetadata(kind: DetailKind, tmdb: string, raw: Raw): TitleDetails {
	const genres = Array.isArray(raw.genres) ? raw.genres.map((g) => (typeof g === 'string' ? g : str((g as Raw)?.name))).filter((g): g is string => !!g) : [];
	const votes = num(raw.vote_count) ?? num(raw.votes);
	const own = str(raw.rating);
	const stars = shownScore(num(raw.vote_average) ?? num(raw.score), votes);
	const rating = own && !/\/\s*10$/.test(own) ? own : stars !== undefined ? `${stars.toFixed(1)}/10` : undefined;
	return {
		kind,
		tmdb,
		name: str(raw.name) ?? str(raw.title) ?? '',
		year: num(raw.year),
		overview: str(raw.overview),
		poster: str(raw.poster),
		backdrop: str(raw.backdrop),
		genres,
		runtime: num(raw.runtime_minutes) ?? num(raw.runtime),
		rating,
		network: str(raw.network),
		cast: list(raw.cast ?? raw.people)
			.map((p) => ({ name: str(p.name) ?? '', role: str(p.character) ?? str(p.role), image: str(p.image) ?? str(p.profile), id: str(p.id) ?? (num(p.id) ? String(p.id) : undefined) }))
			.filter((p) => p.name),
		similar: list(raw.similar)
			.map((s) => {
				const id = str((s.ids as Raw | undefined)?.tmdb) ?? str(s.tmdb) ?? (num(s.id) ? String(s.id) : undefined);
				return { kind: (str(s.kind) === 'show' ? 'show' : kind) as DetailKind, tmdb: id ?? '', name: str(s.name) ?? str(s.title) ?? '', year: num(s.year), poster: str(s.poster) };
			})
			.filter((s) => s.tmdb && s.name),
		titleId: num(raw.title_id)
	};
}

/** fromMovie and fromShow read a lookup answer. */
export function fromMovie(m: LookupMovie): TitleDetails {
	return { kind: 'movie', tmdb: m.ids.tmdb ?? '', name: m.name, year: num(m.year), overview: m.overview || undefined, poster: m.poster, genres: [], runtime: num(m.runtime), cast: [], similar: [], titleId: m.title_id, neverAdd: m.never_add, ask: m.ask, ask4k: m.ask_4k, has4k: m.has_4k, movie: m };
}
export function fromShow(s: LookupShow): TitleDetails {
	return { kind: 'show', tmdb: s.ids.tmdb ?? '', name: s.name, year: num(s.year), overview: s.overview || undefined, poster: s.poster, genres: [], runtime: num(s.runtime), network: s.network, cast: [], similar: [], titleId: s.title_id, neverAdd: s.never_add, ask: s.ask, show: s };
}

type Get = <T>(path: string, signal?: AbortSignal) => Promise<T>;

/** loadDetails asks the metadata endpoint and the lookup by id at once; the lookup's Library and ask facts win.
 * onMeta hears the metadata answer as soon as it arrives, so a page can draw its hero before the lookup is in. */
export async function loadDetails(get: Get, kind: DetailKind, tmdb: string, signal?: AbortSignal, onMeta?: (meta: TitleDetails) => void): Promise<TitleDetails | null> {
	// The metadata answer is asked first: it draws the hero.
	const metaAsk = get<Raw>(`/metadata/${kind}/${encodeURIComponent(tmdb)}`, signal);
	const term = encodeURIComponent(`tmdb:${tmdb}`);
	const lookup: Promise<TitleDetails | null> =
		kind === 'movie'
			? get<{ movies: LookupMovie[] }>(`/titles/lookup?term=${term}`, signal).then((r) => {
					const m = r.movies.find((x) => x.ids.tmdb === tmdb) ?? r.movies[0];
					return m ? fromMovie(m) : null;
				})
			: get<{ shows: LookupShow[] }>(`/shows/lookup?term=${term}`, signal).then((r) => {
					const s = r.shows.find((x) => x.ids.tmdb === tmdb) ?? r.shows[0];
					return s ? fromShow(s) : null;
				});
	// Settled here so a lookup failing while the metadata answer is awaited is not an unhandled rejection.
	const looked = lookup.then(
		(v) => ({ ok: true as const, v }),
		(e: unknown) => ({ ok: false as const, e })
	);
	let meta: TitleDetails | null = null;
	try {
		meta = fromMetadata(kind, tmdb, await metaAsk);
	} catch (e) {
		if (!(e instanceof ApiError) || (e.status !== 404 && e.status !== 405)) throw e;
	}
	if (meta && meta.name) onMeta?.(meta);
	let found: TitleDetails | null = null;
	const r = await looked;
	if (r.ok) found = r.v;
	// A busy or rate-limited lookup still shows the page from the cached details.
	else if (!meta || signal?.aborted) throw r.e;
	if (!meta) return found;
	if (!found) return meta;
	return { ...found, ...meta, name: meta.name || found.name, overview: meta.overview ?? found.overview, poster: meta.poster ?? found.poster, year: meta.year ?? found.year, titleId: found.titleId ?? meta.titleId, neverAdd: found.neverAdd, ask: found.ask, ask4k: found.ask4k, has4k: found.has4k, movie: found.movie, show: found.show };
}

/** fourKOffer says when a member gets a separate 4K Ask next to the main
 * button: after their own plain ask, or for a movie already in the Library
 * without a 4K copy ('asked' once their 4K ask is open). Elsewhere the Ask
 * sheet's 4K box covers it; shows have no 4K ask. */
export function fourKOffer(d: Pick<TitleDetails, 'kind' | 'titleId' | 'neverAdd' | 'ask' | 'ask4k' | 'has4k'>): 'ask' | 'asked' | null {
	if (d.kind !== 'movie' || d.neverAdd || d.has4k) return null;
	if (d.ask4k?.mine) return 'asked';
	return d.titleId || d.ask?.mine ? 'ask' : null;
}

/** metaLine is the year · runtime · genres line under the title. */
export function metaLine(d: Pick<TitleDetails, 'kind' | 'year' | 'runtime' | 'genres' | 'network'>): string[] {
	const out: string[] = [];
	if (d.year) out.push(String(d.year));
	if (d.kind === 'show') out.push('TV show');
	else out.push('Movie');
	if (d.runtime) out.push(d.runtime >= 60 ? `${Math.floor(d.runtime / 60)}h ${d.runtime % 60 ? `${d.runtime % 60}m` : ''}`.trim() : `${d.runtime}m`);
	if (d.network) out.push(d.network);
	return [...out, ...d.genres.slice(0, 3)];
}

/** While a mouse pointer rests on a link to a title page, its hero backdrop
 * (when already known) starts loading. Returns the cleanup. */
export function installHeroPrefetch(preload: (backdrop: string | null | undefined) => void): () => void {
	let timer: ReturnType<typeof setTimeout> | undefined;
	const over = (e: PointerEvent) => {
		if (e.pointerType !== 'mouse') return;
		clearTimeout(timer);
		const a = (e.target as Element | null)?.closest?.('a[href]');
		const m = a?.getAttribute('href')?.match(/\/title\/(movie|show)\/(\d+)/);
		if (m) timer = setTimeout(() => preload(remembered(m[1] as DetailKind, m[2])?.backdrop), 80);
	};
	window.addEventListener('pointerover', over, { capture: true, passive: true });
	return () => {
		clearTimeout(timer);
		window.removeEventListener('pointerover', over, { capture: true });
	};
}
