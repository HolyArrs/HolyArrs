// Music trending: the metadata server's weekly lists (ListenBrainz
// worldwide, Deezer's charts per country) as artist and album rows.
import { api, ApiError, type Client } from './api.ts';
import { to } from './paths.ts';
import type { ExploreCard } from './explore.ts';

export type MusicTrendKind = 'artist' | 'album';

export interface MusicTrendItem {
	rank: number;
	/** The artist, or the album's MusicBrainz release group. */
	mbid: string;
	name: string;
	artist_name?: string;
	artist_mbid?: string;
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
			subtitle: album ? artist : undefined,
			ref: `mbid:${artistMbid}`,
			image: to(`/api/v1/music-trending/art/${a.kind}/${encodeURIComponent(d.mbid)}`),
			badges: [],
			rank: d.rank,
			artist,
			artistMbid
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
