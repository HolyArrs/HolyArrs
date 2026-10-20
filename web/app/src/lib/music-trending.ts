// Music trending: the metadata server's weekly lists (ListenBrainz
// worldwide, Deezer's charts per country) as artist and album rows.
import { api, ApiError, type Client } from './api.ts';
import { to } from './paths.ts';
import type { ExploreCard } from './explore.ts';
import type { CalendarEntry } from './types.ts';

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
