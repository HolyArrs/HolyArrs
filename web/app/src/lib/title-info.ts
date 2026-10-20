// The details a title page reads from GET /metadata/{kind}/{tmdb}: people,
// where to stream, similar titles and the collection, tolerating missing fields.
import type { ProviderItem } from './explore.ts';

export type InfoKind = 'movie' | 'show';


export interface InfoPerson {
	name: string;
	role: string;
	character?: string;
	profile?: string;
}
export interface InfoSeason {
	number: number;
	episodes: number;
	year?: number;
}
export interface InfoService {
	id: number;
	name: string;
	logo?: string;
}
export interface InfoEpisode {
	number: number;
	name?: string;
	overview?: string;
	runtime?: number;
	still?: string;
	airDay?: string;
}
export interface InfoDetails {
	kind: InfoKind;
	tmdb: string;
	name: string;
	year?: number;
	overview?: string;
	backdrop?: string;
	poster?: string;
	logo?: string;
	genres: string[];
	runtime?: number;
	rating?: string;
	cast: InfoPerson[];
	crew: InfoPerson[];
	seasons: InfoSeason[];
	streaming: InfoService[];
	similar: ProviderItem[];
	collection: { name: string; parts: ProviderItem[] } | null;
	titleId?: number;
	head: ProviderItem;
}

type Raw = Record<string, unknown>;
const str = (v: unknown) => (typeof v === 'string' && v.trim() ? v.trim() : undefined);
const pos = (v: unknown) => (typeof v === 'number' && Number.isFinite(v) && v > 0 ? v : undefined);
const arr = (v: unknown): Raw[] => (Array.isArray(v) ? (v.filter((x) => x && typeof x === 'object') as Raw[]) : []);

/** MAX_CAST round faces in a short row. */
export const MAX_CAST = 8;

/** readInfo reads a /metadata/{kind}/{tmdb} answer, tolerating missing fields. */
export function readInfo(kind: InfoKind, tmdb: string, raw: Raw): InfoDetails {
	const people = arr(raw.people).map((p) => ({ name: str(p.name) ?? '', role: str(p.role) ?? 'actor', character: str(p.character), profile: str(p.profile) })).filter((p) => p.name);
	const items = (v: unknown) => arr(v).filter((s) => str(s.tmdb_id) && str(s.name)) as unknown as ProviderItem[];
	const coll = raw.collection && typeof raw.collection === 'object' ? (raw.collection as Raw) : null;
	const parts = coll ? items(coll.parts) : [];
	const titleId = raw.in_library ? pos(raw.title_id) : undefined;
	const name = str(raw.name) ?? '';
	return {
		kind,
		tmdb,
		name,
		year: pos(raw.year),
		overview: str(raw.overview),
		backdrop: str(raw.backdrop),
		poster: str(raw.poster),
		logo: str(raw.logo),
		genres: Array.isArray(raw.genres) ? raw.genres.filter((g): g is string => typeof g === 'string' && !!g) : [],
		runtime: pos(raw.runtime_minutes),
		rating: str(raw.rating),
		cast: people.filter((p) => p.role === 'actor').slice(0, MAX_CAST),
		crew: people.filter((p) => p.role !== 'actor'),
		seasons: arr(raw.seasons)
			.map((s) => ({ number: typeof s.number === 'number' ? s.number : -1, episodes: pos(s.episode_count) ?? 0, year: pos(s.year) }))
			.filter((s) => s.number >= 0 && s.episodes > 0),
		streaming: arr(raw.streaming)
			.map((s) => ({ id: pos(s.id) ?? 0, name: str(s.name) ?? '', logo: str(s.logo) }))
			.filter((s) => s.id && s.name),
		similar: items(raw.similar),
		collection: coll && parts.length > 1 ? { name: str(coll.name) ?? 'Collection', parts } : null,
		titleId,
		head: {
			tmdb_id: tmdb,
			kind,
			name,
			year: pos(raw.year),
			poster: str(raw.poster),
			backdrop: str(raw.backdrop),
			in_library: !!raw.in_library,
			title_id: titleId,
			ask: (raw.ask as ProviderItem['ask']) ?? undefined
		} as ProviderItem
	};
}

/** firstSeason is the season the picker starts on: season 1, else the first real one (specials last). */
export function firstSeason(seasons: InfoSeason[]): number | null {
	if (!seasons.length) return null;
	return (seasons.find((s) => s.number === 1) ?? seasons.find((s) => s.number > 0) ?? seasons[0]).number;
}

export function seasonLabel(n: number): string {
	return n === 0 ? 'Specials' : `Season ${n}`;
}

/** readEpisodes reads a /metadata/show/{tmdb}/seasons/{n} answer. */
export function readEpisodes(raw: Raw): InfoEpisode[] {
	return arr(raw.episodes)
		.map((e) => ({ number: typeof e.number === 'number' ? e.number : 0, name: str(e.name), overview: str(e.overview), runtime: pos(e.runtime), still: str(e.still), airDay: str(e.air_day) }))
		.filter((e) => e.number > 0);
}

export function minutes(m?: number): string {
	if (!m) return '';
	const h = Math.floor(m / 60);
	return h ? `${h}h ${m % 60 ? `${m % 60}m` : ''}`.trim() : `${m}m`;
}

/** facts is the line under the title: year · rating · runtime (movies) or seasons (shows). */
export function facts(d: Pick<InfoDetails, 'kind' | 'year' | 'rating' | 'runtime' | 'seasons'>): string[] {
	// The type leads, as on every card: "Show · 4 seasons · 2024 · TV-MA".
	const out: string[] = [];
	if (d.kind === 'movie') {
		const r = minutes(d.runtime);
		if (r) out.push(r);
	} else {
		const n = d.seasons.filter((s) => s.number > 0).length;
		if (n) out.push(n === 1 ? '1 season' : `${n} seasons`);
	}
	if (d.year) out.push(String(d.year));
	if (d.rating) out.push(d.rating);
	return out.length ? [d.kind === 'movie' ? 'Movie' : 'Show', ...out] : out;
}

