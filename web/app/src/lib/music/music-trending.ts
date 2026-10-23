// Music trending: the metadata server's weekly lists (ListenBrainz
// worldwide, Deezer's charts per country) as artist and album rows.
import { api, ApiError, type Client } from '../api.ts';
import type { Fresh } from '../fresh.ts';
import { to } from '../paths.ts';
import type { ExploreCard } from '../explore.ts';
import type { CalendarEntry } from '../types.ts';

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

export interface MusicTrendCard extends ExploreCard {
	rank: number;
	/** The artist the card adds or opens (the album's artist for albums). */
	artist: string;
	artistMbid: string;
	/** Albums are square, artists round. */
	shape: 'square' | 'round';
}

/** listeners: "12K listeners this week" (social proof on chart rows), or ''. */
export function listeners(n: number | undefined): string {
	if (!n || n <= 0) return '';
	const short = n >= 1e6 ? `${(n / 1e6).toFixed(n >= 1e7 ? 0 : 1)}M` : n >= 1e3 ? `${(n / 1e3).toFixed(n >= 1e4 ? 0 : 1)}K` : String(n);
	return `${short.replace('.0', '')} ${n === 1 ? 'listen' : 'listens'} this week`;
}

/**
 * musicTrendCards turns an answer into poster cards. Both kinds point at an
 * artist (ref mbid:<artist>), so a card whose artist is in the Library opens
 * the artist page; albums without a known artist are left out. Pictures come
 * through HolyArrs' artwork cache.
 */
export function musicTrendCards(a: MusicTrendAnswer): MusicTrendCard[] {
	const out: MusicTrendCard[] = [];
	for (const d of a.items) {
		const album = a.kind === 'album';
		const artistMbid = album ? (d.artist_mbid ?? '') : d.mbid;
		const artist = album ? (d.artist_name ?? '') : d.name;
		if (!d.mbid || !artistMbid || !artist) continue;
		out.push({
			key: `${a.kind}:${d.mbid}`,
			kind: 'artist',
			name: d.name,
			subtitle: [album ? artist : '', listeners(d.listen_count)].filter(Boolean).join(' · ') || undefined,
			ref: `mbid:${artistMbid}`,
			image: to(`/api/v1/music-trending/art/${a.kind}/${encodeURIComponent(d.mbid)}`),
			badges: [],
			rank: d.rank,
			artist,
			artistMbid,
			shape: album ? 'square' : 'round'
		});
	}
	return out;
}

/** fetchMusicTrend reads one list; a country without one falls back to global; null when there is none. */
export async function fetchMusicTrend(kind: MusicTrendKind, region: string, client: Pick<Client, 'get'> = api, signal?: AbortSignal): Promise<MusicTrendAnswer | null> {
	for (const r of region === 'global' ? ['global'] : [region, 'global']) {
		try {
			const a = await client.get<MusicTrendAnswer>(`/music-trending?kind=${kind}&region=${encodeURIComponent(r)}`, signal);
			return a.items?.length ? a : null;
		} catch (e) {
			if (!(e instanceof ApiError && e.status === 404)) return null;
		}
	}
	return null;
}

/** NewAlbum is an album out in the last 60 days from an artist in the Library. */
export interface NewAlbum {
	key: string;
	titleId: number;
	artist: string;
	album: string;
	date: string;
	image: string | null;
}

/** newFromYourArtists: Library artists' albums out in the last `days` days, newest first (one per album). */
export function newFromYourArtists(entries: CalendarEntry[], now = new Date(), days = 60): NewAlbum[] {
	const from = now.getTime() - days * 86400000;
	const seen = new Set<string>();
	return entries
		.filter((e) => e.kind === 'album' && e.album && new Date(e.date).getTime() >= from && new Date(e.date).getTime() <= now.getTime())
		.sort((a, b) => b.date.localeCompare(a.date))
		.filter((e) => {
			const k = `${e.title_id}:${e.part_id ?? e.album}`;
			if (seen.has(k)) return false;
			seen.add(k);
			return true;
		})
		.map((e) => ({ key: `new-${e.title_id}-${e.part_id ?? e.album}`, titleId: e.title_id, artist: e.name, album: e.album!, date: e.date, image: e.poster ?? null }));
}

// ---- Explore → Music: every row, built by the server ----

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
	/** A genre tile's colour index (flag music_rows_v2). */
	hue?: number;
	/** A genre tile's slideshow of artist photos (image is the first). */
	images?: string[];
}

export interface MusicRowData {
	id: string;
	type: string;
	title: string;
	reason: string;
	shape: 'square' | 'round' | 'tile' | 'song';
	items: MusicCardData[];
}

export interface MusicExploreAnswer {
	/** The music_rows_v2 rows (genre tiles and song lines, no genre chips). */
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
	/** A Music genre page's own tile (name, hue, art) for its header band. */
	tile?: MusicCardData;
}

/** fetchMusicGenre reads a Music genre page's rows (Music only; Movies and Shows keep their own genre pages). */
export async function fetchMusicGenre(slug: string, region: string, client: Pick<Client, 'get'> = api, signal?: AbortSignal): Promise<MusicExploreAnswer> {
	return client.get<MusicExploreAnswer>(`/music/genre/${encodeURIComponent(slug)}?${new URLSearchParams({ region })}`, signal);
}

/** fetchMusicExplore reads the Music rows for a genre chip ('' is all) and chart region. */
export async function fetchMusicExplore(genre: string, region: string, client: Pick<Client, 'get'> = api, signal?: AbortSignal, hour = new Date().getHours()): Promise<MusicExploreAnswer> {
	const q = new URLSearchParams({ region, hour: String(hour) });
	if (genre) q.set('genre', genre);
	return client.get<MusicExploreAnswer>(`/explore/music?${q}`, signal);
}

const askWords: Record<string, string> = { asked: 'Asked', approved: 'Approved, on the way', turned_down: 'Turned down', available: 'Arrived' };

function shortDay(d: string): string {
	const t = new Date(`${d}T00:00:00Z`);
	return Number.isNaN(t.getTime()) ? d : t.toLocaleDateString(undefined, { day: 'numeric', month: 'short', timeZone: 'UTC' });
}

/**
 * releaseWords says when an album comes or came out: "Out Sep 25" ahead,
 * "Out now" within 14 days after, "Released Sep 25" after that.
 */
export function releaseWords(date: string, now = new Date()): string {
	const t = new Date(`${date}T00:00:00Z`);
	if (Number.isNaN(t.getTime())) return '';
	const today = Date.UTC(now.getFullYear(), now.getMonth(), now.getDate());
	const days = Math.round((today - t.getTime()) / 86400000);
	if (days < 0) return `Out ${shortDay(date)}`;
	if (days <= 14) return 'Out now';
	return `Released ${shortDay(date)}`;
}

/** musicCardNote is a card's one-line reason, by the row it is in. */
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
			return [by, c.date ? `Arrived ${shortDay(c.date)}` : ''].filter(Boolean).join(' · ');
		case 'new_from':
			return [by, c.date ? releaseWords(c.date) : ''].filter(Boolean).join(' · ');
		case 'your_asks':
			return askWords[c.state ?? ''] ?? '';
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

/** musicStatus is a card's state chip (music_rows_v2): have, asked or approved. */
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

/** genreHref opens a genre tile: Search, music scope, until genre pages exist. */
export function genreHref(c: Pick<MusicCardData, 'name'> & Partial<Pick<MusicCardData, 'kind' | 'mbid'>>): string {
	if (c.kind === 'genre' && c.mbid) return to(`/music/genre/${encodeURIComponent(c.mbid)}`);
	return to(`/search?q=${encodeURIComponent(c.name)}&kind=music`);
}

/** songAskHref: a song line asks for its album, so it opens the artist (or a search for the song). */
export function songAskHref(c: Pick<MusicCardData, 'artist_mbid' | 'artist' | 'name'>): string {
	if (c.artist_mbid) return to(`/music/artist/${c.artist_mbid}`);
	return to(`/search?q=${encodeURIComponent(`${c.artist ?? ''} ${c.name}`.trim())}&kind=music`);
}

/** musicCardHref: an album opens its album page, an artist the artist page (any artist, in the Library or not); without MusicBrainz ids the Library page or the artist search. */
export function musicCardHref(c: MusicCardData): string {
	if (c.kind === 'album' && c.mbid && c.artist_mbid) return to(`/music/album/${c.mbid}?artist=${c.artist_mbid}`);
	const artist = c.kind === 'artist' ? c.mbid || c.artist_mbid : c.artist_mbid;
	if (artist) return to(`/music/artist/${artist}`);
	if (c.title_id && c.part_id && c.kind === 'album') return to(`/library/${c.title_id}/albums/${c.part_id}`);
	if (c.title_id) return to(`/library/${c.title_id}`);
	return to(`/search?q=${encodeURIComponent(c.artist || c.name)}&kind=music`);
}

/** musicImage puts the server's picture path under the app's base path. */
export function musicImage(c: Pick<MusicCardData, 'image'>): string | null {
	return c.image ? to(c.image) : null;
}
