// Title details (backdrop, story, runtime, rating) for asks shown big on the Requests page. Asked once per title while the app is open, from the install's own metadata endpoint (the same answer the title page uses).

import { detailKey } from './board.ts';
import { readInfo, type AskInfo, type EpisodeInfo } from './inbox.ts';
import { musicDefaults, type ArtistAlbum, type MusicKinds } from './ask-parts.ts';
import { fromMetadata, type TitleDetails } from '../metadata/index.ts';
import { appPicture, type ArtistCoreData } from '../music/index.ts';
import { api } from '../core/api.ts';
import type { Ask } from '../core/types.ts';

/** An ask's lists the server stored when it was asked for: episodes by season number, songs by album id. */
export interface StoredParts {
	seasons: Record<string, EpisodeInfo[]>;
	albums: Record<string, AlbumSong[]>;
}

/** A song of an album, as the album page lists it. */
export interface AlbumSong {
	disc: number;
	number: number;
	name: string;
	length_ms?: number;
	recording_id?: string;
}

/** What the Requests panel shows of an artist: the round photo, the banner pictures and the albums (release groups). */
export interface AskArtist {
	image: string | null;
	backgrounds: string[];
	albums: ArtistAlbum[] | null;
}

const got = $state<Record<string, TitleDetails | null>>({});
const info = $state<Record<string, AskInfo | null>>({});
const eps = $state<Record<string, EpisodeInfo[] | null>>({});
const asked = new Set<string>();
const askedEps = new Set<string>();

/** What is known for an ask: details, null (none, or not yet), undefined (music or no id). */
export function askDetails(a: Pick<Ask, 'kind' | 'tmdb_id'>): TitleDetails | null | undefined {
	const k = detailKey(a);
	return k ? (got[k] ?? null) : undefined;
}

/** The seasons and release dates for an ask's summary (null until known or for music). */
export function askInfo(a: Pick<Ask, 'kind' | 'tmdb_id'>): AskInfo | null {
	const k = detailKey(a);
	return k ? (info[k] ?? null) : null;
}

/** Asks for an ask's details once. */
export function wantDetails(a: Pick<Ask, 'kind' | 'tmdb_id'>): void {
	const k = detailKey(a);
	if (!k || asked.has(k)) return;
	asked.add(k);
	const kind = a.kind === 'show' ? 'show' : 'movie';
	api
		.get<Record<string, unknown>>(`/metadata/${kind}/${encodeURIComponent(a.tmdb_id)}`)
		.then((raw) => {
			got[k] = fromMetadata(kind, a.tmdb_id, raw);
			info[k] = readInfo(raw);
		})
		.catch(() => {
			got[k] = null;
			info[k] = null;
		});
}

/** One season's episodes (null until known), asked once. */
export function seasonEpisodes(tmdb: string, season: number): EpisodeInfo[] | null {
	const k = `${tmdb}:${season}`;
	if (!askedEps.has(k) && tmdb && season > 0) {
		askedEps.add(k);
		api
			.get<{ episodes?: EpisodeInfo[] }>(`/metadata/show/${encodeURIComponent(tmdb)}/seasons/${season}`)
			.then((r) => (eps[k] = Array.isArray(r.episodes) ? r.episodes : []))
			.catch(() => (eps[k] = []));
	}
	return eps[k] ?? null;
}

const stored = $state<Record<number, StoredParts | null>>({});
const askedStored = new Set<number>();

/** An ask's stored lists (null until known), asked once from the install; TV and music asks only. */
export function storedParts(a: Pick<Ask, 'id' | 'kind'> | null | undefined): StoredParts | null {
	if (!a || (a.kind !== 'show' && a.kind !== 'artist')) return null;
	if (!askedStored.has(a.id)) {
		askedStored.add(a.id);
		api
			.get<StoredParts>(`/asks/${a.id}/parts`)
			.then((r) => (stored[a.id] = { seasons: r.seasons ?? {}, albums: r.albums ?? {} }))
			.catch(() => (stored[a.id] = { seasons: {}, albums: {} }));
	}
	return stored[a.id] ?? null;
}

/** A song's id for ticks: its recording, else disc and number. */
export const songKey = (t: AlbumSong) => t.recording_id || `${t.disc}-${t.number}`;

const songs = $state<Record<string, AlbumSong[] | null>>({});
const askedSongs = new Set<string>();
const songQueue: (() => Promise<void>)[] = [];
let songsRunning = 0;

/** An album's songs (null until known), asked once through the album page's answer, at most 3 albums at a time. */
export function albumSongs(album: string, artist: string): AlbumSong[] | null {
	if (album && artist && !askedSongs.has(album)) {
		askedSongs.add(album);
		// The album answer says incomplete while the server still fetches the tracklist: ask again shortly (a few times).
		const ask = (tries: number): Promise<void> =>
			api
				.get<{ tracks?: AlbumSong[]; incomplete?: boolean }>(`/music/albums/${encodeURIComponent(album)}?artist=${encodeURIComponent(artist)}`)
				.then((r) => {
					if (r.incomplete && !r.tracks?.length && tries < 5) {
						setTimeout(() => {
							songQueue.push(() => ask(tries + 1));
							nextSongs();
						}, 2000);
						return;
					}
					songs[album] = Array.isArray(r.tracks) ? r.tracks : [];
				})
				.catch(() => void (songs[album] = []));
		songQueue.push(() => ask(0));
		nextSongs();
	}
	return songs[album] ?? null;
}

const artists = $state<Record<string, AskArtist>>({});
const askedArtists = new Set<string>();
const empty: AskArtist = { image: null, backgrounds: [], albums: null };
let kinds = $state<MusicKinds>(musicDefaults);
let kindsAsked = false;

/** An artist ask's MusicBrainz id. */
export function artistIdOf(a: Pick<Ask, 'kind' | 'ref' | 'artist_id'>): string {
	if (a.kind !== 'artist') return '';
	return a.artist_id || (a.ref?.startsWith('mbid:') ? a.ref.slice(5) : '');
}

/** An artist ask's round photo (the artist's own picture, never an album cover). */
export function artistPhoto(a: Pick<Ask, 'kind' | 'ref' | 'artist_id' | 'poster'>): string | null {
	const id = artistIdOf(a);
	return artists[id]?.image ?? (id ? appPicture(`/api/v1/music-trending/art/artist/${id}`) : appPicture(a.poster));
}

/** What is known of an ask's artist (null until asked); asks once, the same answers the artist page uses. */
export function askArtist(a: Pick<Ask, 'kind' | 'ref' | 'artist_id'>): AskArtist | null {
	const id = artistIdOf(a);
	if (!id) return null;
	if (!askedArtists.has(id)) {
		askedArtists.add(id);
		// Read inside the page's derived values: the store is only written once an answer lands.
		const cur = () => artists[id] ?? empty;
		api
			.get<ArtistCoreData>(`/music/artists/${encodeURIComponent(id)}?part=core`)
			.then((c) => (artists[id] = { ...cur(), image: appPicture(c.image), albums: (c.albums ?? []).map((x) => ({ ...x, image: appPicture(x.image) ?? '' })) }))
			.catch(() => (artists[id] = { ...cur(), albums: [] }));
		api
			.get<{ backgrounds?: string[] }>(`/extras/artist/${encodeURIComponent(id)}`)
			.then((x) => (artists[id] = { ...cur(), backgrounds: (x.backgrounds ?? []).map((u) => (u.startsWith('/') ? (appPicture(u) ?? u) : u)) }))
			.catch(() => {});
	}
	return artists[id] ?? empty;
}

/** The album kinds a whole-artist ask takes (the music add defaults; the built-in ones until known). */
export function musicKinds(): MusicKinds {
	if (!kindsAsked) {
		kindsAsked = true;
		api
			.get<MusicKinds>('/settings/music')
			.then((r) => (kinds = { album_types: r.album_types ?? musicDefaults.album_types, secondary_types: r.secondary_types ?? musicDefaults.secondary_types }))
			.catch(() => {});
	}
	return kinds;
}

function nextSongs() {
	while (songsRunning < 3 && songQueue.length) {
		const job = songQueue.shift()!;
		songsRunning++;
		void job().finally(() => {
			songsRunning--;
			nextSongs();
		});
	}
}
