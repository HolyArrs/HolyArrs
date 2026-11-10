// Reads loosely typed TMDB detail payloads into safe typed shapes.
import type { ProviderItem } from './title-cards.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';

export type InfoKind = 'movie' | 'show';

export interface InfoPerson {
	name: string;
	role: string;
	character?: string;
	profile?: string;
	id?: string;
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
	moods: string[];
	originalLanguage?: string;
	originCountry: string[];
	status?: string;
	tagline?: string;
	originalTitle?: string;
	budget?: number;
	revenue?: number;
	studios: string[];
	showType?: string;
	episodeRuntime?: number;
	spokenLanguages: string[];
	dates: { theaters?: string; digital?: string; physical?: string; firstAir?: string; lastAir?: string };
	nextEpisode?: { season: number; episode: number; name?: string; airDate: string };
	keywords: string[];
	titleId?: number;
	head: ProviderItem;
}

type Raw = Record<string, unknown>;

// Cast is capped so the strip stays short.
export const MAX_CAST = 8;

// Every field is checked because the payload is untrusted; bad entries are dropped.
export function readInfo(kind: InfoKind, tmdb: string, raw: Raw): InfoDetails {
	const people = arr(raw.people).map((p) => ({ name: str(p.name) ?? '', role: str(p.role) ?? 'actor', character: str(p.character), profile: str(p.profile), id: str(p.id) })).filter((p) => p.name);
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
		collection: coll && parts.length > 1 ? { name: str(coll.name) ?? t('ui.titleInfo.collection'), parts } : null,
		moods: words(raw.moods),
		originalLanguage: str(raw.original_language),
		originCountry: words(raw.origin_country),
		status: str(raw.status),
		tagline: str(raw.tagline),
		originalTitle: str(raw.original_title),
		budget: pos(raw.budget),
		revenue: pos(raw.revenue),
		studios: words(raw.studios),
		showType: str(raw.show_type),
		episodeRuntime: typeof raw.episode_runtime === 'number' && raw.episode_runtime > 0 ? raw.episode_runtime : undefined,
		spokenLanguages: words(raw.spoken_languages),
		dates: readDates(raw.dates),
		nextEpisode: readNext(raw.next_episode),
		keywords: words(raw.keywords),
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

// 135 -> "2h 15m".
export function minutes(m?: number): string {
	if (!m) return '';
	const h = Math.floor(m / 60);
	return h ? `${h}h ${m % 60 ? `${m % 60}m` : ''}`.trim() : `${m}m`;
}

// Header chips; empty when there is nothing to show.
export function facts(d: Pick<InfoDetails, 'kind' | 'year' | 'rating' | 'runtime' | 'seasons'>): string[] {
	const out: string[] = [];
	if (d.kind === 'movie') {
		const r = minutes(d.runtime);
		if (r) out.push(r);
	} else {
		const n = d.seasons.filter((s) => s.number > 0).length;
		if (n) out.push(n === 1 ? t('ui.titleInfo.seasonOne') : t('ui.titleInfo.seasonOther', { count: n }));
	}
	if (d.year) out.push(String(d.year));
	if (d.rating) out.push(d.rating);
	return out.length ? [d.kind === 'movie' ? t('ui.titleInfo.movie') : t('ui.titleInfo.show'), ...out] : out;
}

// Small guards that turn unknown values into a clean value or undefined.
const str = (v: unknown) => (typeof v === 'string' && v.trim() ? v.trim() : undefined);
const pos = (v: unknown) => (typeof v === 'number' && Number.isFinite(v) && v > 0 ? v : undefined);
const words = (v: unknown): string[] => (Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string' && !!x.trim()) : []);
const arr = (v: unknown): Raw[] => (Array.isArray(v) ? (v.filter((x) => x && typeof x === 'object') as Raw[]) : []);

// Keeps only the YYYY-MM-DD part of a date.
const day = (v: unknown) => (typeof v === 'string' && /^\d{4}-\d{2}-\d{2}/.test(v) ? v.slice(0, 10) : undefined);
function readDates(v: unknown): InfoDetails['dates'] {
	const d = v && typeof v === 'object' ? (v as Raw) : {};
	return { theaters: day(d.theaters), digital: day(d.digital), physical: day(d.physical), firstAir: day(d.first_air), lastAir: day(d.last_air) };
}
function readNext(v: unknown): InfoDetails['nextEpisode'] {
	if (!v || typeof v !== 'object') return undefined;
	const e = v as Raw;
	const airDate = day(e.air_date);
	return airDate ? { season: typeof e.season === 'number' ? e.season : 0, episode: typeof e.episode === 'number' ? e.episode : 0, name: str(e.name), airDate } : undefined;
}
