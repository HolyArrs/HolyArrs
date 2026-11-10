import type { Fresh } from '#lib/ui/fresh.ts';
import { versioned } from '../core/pictures.ts';
import { api, type Client } from '../core/api.ts';
import { to } from '../core/paths.ts';
import { t, appLocale } from '#lib/core/i18n/i18n.svelte.ts';

// Music trending: the metadata server's weekly lists (ListenBrainz worldwide, Deezer's charts per country) as artist and album rows.

export type MusicTrendKind = 'artist' | 'album';

export interface MusicTrendItem {
	rank: number;
	/** The artist, or the album's MusicBrainz release group. */
	mbid: string;
	name: string;
	artist_name?: string;
	artist_mbid?: string;
	listen_count?: number;
}

export interface MusicTrendAnswer {
	kind: MusicTrendKind;
	region: string;
	source: string;
	regions: string[] | null;
	items: MusicTrendItem[];
}

/** "12K listeners this week" (social proof on chart rows), or ''. */
export function listeners(n: number | undefined): string {
	if (!n || n <= 0) return '';
	const short = n >= 1e6 ? `${(n / 1e6).toFixed(n >= 1e7 ? 0 : 1)}M` : n >= 1e3 ? `${(n / 1e3).toFixed(n >= 1e4 ? 0 : 1)}K` : String(n);
	return n === 1 ? t('music.listenThisWeek', { n: short.replace('.0', '') }) : t('music.listensThisWeek', { n: short.replace('.0', '') });
}

export interface MusicCardData {
	key: string;
	kind: 'album' | 'artist' | 'song' | 'genre';
	mbid: string;
	name: string;
	artist?: string;
	artist_mbid?: string;
	title_id?: number;
	part_id?: number;
	image?: string;
	/** The artist's picture, on hero candidates (shown blurred when no album cover loads). */
	artist_image?: string;
	/** A Spotlight card's label above its title (New album, Climbing). */
	kicker?: string;
	/** YYYY-MM-DD: when it arrived, comes out or was asked for. */
	date?: string;
	state?: string;
	rank?: number;
	listen_count?: number;
	upcoming?: boolean;
	genres: string[];
	/** The server freshness strip (Recently Added, New Release). */
	fresh?: Fresh | null;
	/** The card's own reason line, when the server gives one. */
	note?: string;
	/** Explicit lyrics: the "E" mark. */
	explicit?: boolean;
	/** A song line's song (the line asks for its album). */
	track?: string;
	/** A genre tile's colour index . */
	hue?: number;
	/** A genre tile's slideshow of artist photos (image is the first). */
	images?: string[];
	/** A Browse tile's tilted album cover (flag music_lists_v1). */
	cover?: string;
	/** A Browse tile's own page when it is not a genre page (row templates). */
	href?: string;
}

export interface MusicRowData {
	id: string;
	type: string;
	title: string;
	reason: string;
	shape: 'square' | 'round' | 'tile' | 'song' | 'grid' | 'wide';
	items: MusicCardData[];
}

export interface MusicExploreAnswer {
	/** Always true now: genre tiles and song lines, no genre chips. */
	v2?: boolean;
	kid: boolean;
	cold: boolean;
	genres: string[];
	genre: string;
	region?: string;
	regions?: string[];
	hero: MusicCardData | null;
	/** The hero's stand-ins, next newest first: the page shows the first whose cover loads. */
	hero_alts?: MusicCardData[];
	rows: MusicRowData[];
	/** Still filling in on the server: the page asks again quietly (reaskWhileFilling). */
	partial?: boolean;
	/** Row templates: the page has too little music yet (no rows); similar pages to try. */
	thin?: boolean;
	similar?: { key: string; name: string; href: string }[];
	/** A Music genre page's own tile (name, hue, art) for its header band. */
	tile?: MusicCardData;
	/** The cursor for the next 6 rows ('' or absent: no more). */
	next?: string;
	/** The day's planned row count. */
	total?: number;
}

/** Reads a Music genre page's rows (Music only; Movies and Shows keep their own genre pages). */
export async function fetchMusicGenre(slug: string, region: string, client: Pick<Client, 'get'> = api, signal?: AbortSignal): Promise<MusicExploreAnswer> {
	return client.get<MusicExploreAnswer>(`/music/genre/${encodeURIComponent(slug)}?${new URLSearchParams({ region })}`, signal);
}

/** A Music Browse page kind (music_lists_v1 row templates): genre pages keep their route; moods, seasons, decades and activities have their own. */
export type MusicPageKind = 'genre' | 'mood' | 'season' | 'decade' | 'activity';

/** Reads one Music Browse page's rows. */
export async function fetchMusicPage(kind: MusicPageKind, slug: string, region: string, client: Pick<Client, 'get'> = api, signal?: AbortSignal): Promise<MusicExploreAnswer> {
	if (kind === 'genre') return fetchMusicGenre(slug, region, client, signal);
	return client.get<MusicExploreAnswer>(`/music/browse/${kind}/${encodeURIComponent(slug)}?${new URLSearchParams({ region })}`, signal);
}

/** Reads the Music rows for a genre chip ('' is all) and chart region. */
export async function fetchMusicExplore(genre: string, region: string, client: Pick<Client, 'get'> = api, signal?: AbortSignal, hour = new Date().getHours()): Promise<MusicExploreAnswer> {
	const q = new URLSearchParams({ region, hour: String(hour) });
	if (genre) q.set('genre', genre);
	return client.get<MusicExploreAnswer>(`/explore/music?${q}`, signal);
}

/** Reads the "Rising today" row (round artist cards); null when there is none today or the server did not answer. */
export async function fetchRisingToday(client: Pick<Client, 'get'> = api, signal?: AbortSignal): Promise<MusicRowData | null> {
	try {
		const r = await client.get<MusicRowData>('/explore/music/rising', signal);
		return r?.items?.length ? r : null;
	} catch {
		return null;
	}
}

/** Puts the rising row after the trending artists (else third), once. */
export function withRising<T extends { id: string; type: string }>(rows: T[], rising: T | null): T[] {
	if (!rising || rows.some((r) => r.id === rising.id)) return rows;
	const at = rows.findIndex((r) => r.type === 'trending_artists');
	const i = at >= 0 ? at + 1 : Math.min(2, rows.length);
	return [...rows.slice(0, i), rising, ...rows.slice(i)];
}

/** Reads the next 6 Music rows of the day's plan (music_rows_v7). */
export async function fetchMusicExploreMore(cursor: string, region: string, client: Pick<Client, 'get'> = api, signal?: AbortSignal): Promise<MusicExploreAnswer> {
	return client.get<MusicExploreAnswer>(`/explore/music?${new URLSearchParams({ region, cursor })}`, signal);
}

const askWords = (): Record<string, string> => ({ asked: t('music.asked'), approved: t('music.approvedOnTheWay'), turned_down: t('music.turnedDown'), available: t('music.arrived') });

function shortDay(d: string): string {
	const day = new Date(`${d}T00:00:00Z`);
	return Number.isNaN(day.getTime()) ? d : day.toLocaleDateString(appLocale(), { day: 'numeric', month: 'short', timeZone: 'UTC' });
}

/** When an album comes or came out: "Out Sep 25" ahead, "Out now" within 14 days after, "Released Sep 25" after that. */
export function releaseWords(date: string, now = new Date()): string {
	const at = new Date(`${date}T00:00:00Z`);
	if (Number.isNaN(at.getTime())) return '';
	const today = Date.UTC(now.getFullYear(), now.getMonth(), now.getDate());
	const days = Math.round((today - at.getTime()) / 86400000);
	if (days < 0) return t('music.out', { date: shortDay(date) });
	if (days <= 14) return t('music.outNow');
	return t('music.released', { date: shortDay(date) });
}

/** A card's one-line reason, by the row it is in. */
export function musicCardNote(row: Pick<MusicRowData, 'type'>, c: MusicCardData): string {
	const by = c.kind === 'album' && c.artist ? c.artist : '';
	if (c.note) return c.note;
	switch (row.type) {
		case 'fresh':
			return [by, c.date ? releaseWords(c.date) : ''].filter(Boolean).join(' · ');
		case 'rising':
			return listeners(c.listen_count);
		case 'classic':
		case 'decade':
			return [by, c.date ? c.date.slice(0, 4) : ''].filter(Boolean).join(' · ');
		case 'just_arrived':
			return [by, c.date ? t('music.arrived2', { date: shortDay(c.date) }) : ''].filter(Boolean).join(' · ');
		case 'new_from':
			return [by, c.date ? releaseWords(c.date) : ''].filter(Boolean).join(' · ');
		case 'your_asks':
			return askWords()[c.state ?? ''] ?? '';
		case 'top_albums':
		case 'trending_artists':
		case 'popular_genre':
		case 'popular_country':
			return [by, listeners(c.listen_count)].filter(Boolean).join(' · ');
		case 'missing':
			return [by, c.date ? c.date.slice(0, 4) : ''].filter(Boolean).join(' · ');
		case 'coming_soon':
			return [by, c.date ? releaseWords(c.date) : ''].filter(Boolean).join(' · ');
	}
	return by;
}

/** A card's state chip (music_rows_v2): have, asked or approved. */
export function musicStatus(c: Pick<MusicCardData, 'state'>): 'in_library' | 'requested' | 'approved' | undefined {
	switch (c.state) {
		case 'have':
			return 'in_library';
		case 'asked':
			return 'requested';
		case 'approved':
			return 'approved';
	}
	return undefined;
}

/** Opens a genre tile: Search, music scope, until genre pages exist. */
export function genreHref(c: Pick<MusicCardData, 'name'> & Partial<Pick<MusicCardData, 'kind' | 'mbid'>>): string {
	if (c.kind === 'genre' && c.mbid) return to(`/music/genre/${encodeURIComponent(c.mbid)}`);
	return to(`/search?q=${encodeURIComponent(c.name)}&kind=music`);
}

/** A song line asks for its album, so it opens the artist (or a search for the song). */
export function songAskHref(c: Pick<MusicCardData, 'artist_mbid' | 'artist' | 'name'>): string {
	if (c.artist_mbid) return to(`/music/artist/${c.artist_mbid}`);
	return to(`/search?q=${encodeURIComponent(`${c.artist ?? ''} ${c.name}`.trim())}&kind=music`);
}

/** An album opens its album page, an artist the artist page (any artist, in the Library or not); without MusicBrainz ids the Library page or the artist search. */
export function musicCardHref(c: MusicCardData): string {
	if (c.kind === 'album' && c.mbid && c.artist_mbid) return to(`/music/album/${c.mbid}?artist=${c.artist_mbid}`);
	const artist = c.kind === 'artist' ? c.mbid || c.artist_mbid : c.artist_mbid;
	if (artist) return to(`/music/artist/${artist}`);
	if (c.title_id && c.part_id && c.kind === 'album') return to(`/library/${c.title_id}/albums/${c.part_id}`);
	if (c.title_id) return to(`/library/${c.title_id}`);
	return to(`/search?q=${encodeURIComponent(c.artist || c.name)}&kind=music`);
}

/** Puts the server's picture path under the app's base path. */
export function musicImage(c: Pick<MusicCardData, 'image'>): string | null {
	return c.image ? to(c.image.startsWith('/') ? versioned(c.image) : c.image) : null;
}
