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
	return { kind: 'movie', tmdb: m.ids.tmdb ?? '', name: m.name, year: num(m.year), overview: m.overview || undefined, poster: m.poster, genres: [], runtime: num(m.runtime), cast: [], similar: [], titleId: m.title_id, neverAdd: m.never_add, ask: m.ask, movie: m };
}
export function fromShow(s: LookupShow): TitleDetails {
	return { kind: 'show', tmdb: s.ids.tmdb ?? '', name: s.name, year: num(s.year), overview: s.overview || undefined, poster: s.poster, genres: [], runtime: num(s.runtime), network: s.network, cast: [], similar: [], titleId: s.title_id, neverAdd: s.never_add, ask: s.ask, show: s };
}

type Get = <T>(path: string, signal?: AbortSignal) => Promise<T>;

/** loadDetails asks the metadata endpoint, then the lookup by id; the lookup's Library and ask facts win. */
export async function loadDetails(get: Get, kind: DetailKind, tmdb: string, signal?: AbortSignal): Promise<TitleDetails | null> {
	let meta: TitleDetails | null = null;
	try {
		meta = fromMetadata(kind, tmdb, await get<Raw>(`/metadata/${kind}/${encodeURIComponent(tmdb)}`, signal));
	} catch (e) {
		if (!(e instanceof ApiError) || (e.status !== 404 && e.status !== 405)) throw e;
	}
	const term = encodeURIComponent(`tmdb:${tmdb}`);
	let found: TitleDetails | null = null;
	if (kind === 'movie') {
		const r = await get<{ movies: LookupMovie[] }>(`/titles/lookup?term=${term}`, signal);
		const m = r.movies.find((x) => x.ids.tmdb === tmdb) ?? r.movies[0];
		if (m) found = fromMovie(m);
	} else {
		const r = await get<{ shows: LookupShow[] }>(`/shows/lookup?term=${term}`, signal);
		const s = r.shows.find((x) => x.ids.tmdb === tmdb) ?? r.shows[0];
		if (s) found = fromShow(s);
	}
	if (!meta) return found;
	if (!found) return meta;
	return { ...found, ...meta, name: meta.name || found.name, overview: meta.overview ?? found.overview, poster: meta.poster ?? found.poster, year: meta.year ?? found.year, titleId: found.titleId ?? meta.titleId, neverAdd: found.neverAdd, ask: found.ask, movie: found.movie, show: found.show };
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
