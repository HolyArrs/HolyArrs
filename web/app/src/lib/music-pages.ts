// The artist and album pages: types, links and the small rules they share.
import { to } from './paths.ts';

export type PageAlbumState = 'complete' | 'partial' | 'missing' | 'downloading' | 'not_out_yet' | 'not_included' | '';
export type Shelf = 'albums' | 'singles' | 'live' | 'other';

export interface PageAlbum {
	mbid: string;
	name: string;
	type: string;
	secondary_types: string[];
	release_date?: string;
	disambiguation?: string;
	shelf: Shelf;
	image: string;
	explicit?: boolean;
	upcoming?: boolean;
	state?: PageAlbumState;
	part_id?: number;
	following?: boolean;
}

export interface PageSong {
	title: string;
	length_ms: number;
	explicit?: boolean;
	album: string;
	album_mbid?: string;
	image?: string;
}

export interface PageArtist {
	mbid: string;
	name: string;
	image: string;
	note?: string;
}

export interface WikiBio {
	title: string;
	extract: string;
	url: string;
	license_url: string;
}

/** ArtistCoreData is the hero's quick answer (?part=core); the full answer extends it. */
export interface ArtistCoreData {
	mbid: string;
	name: string;
	type?: string;
	country?: string;
	disambiguation?: string;
	status?: string;
	image: string;
	title_id?: number;
	bio: WikiBio | null;
	kid: boolean;
}

export interface ArtistPageData {
	mbid: string;
	name: string;
	type?: string;
	country?: string;
	disambiguation?: string;
	status?: string;
	genres: string[];
	image: string;
	title_id?: number;
	have: number;
	total: number;
	bio: WikiBio | null;
	albums: PageAlbum[];
	latest: PageAlbum | null;
	top: PageSong[];
	similar: PageArtist[];
	related: PageArtist[];
	kid: boolean;
}

export interface PageTrack {
	disc: number;
	number: number;
	name: string;
	length_ms?: number;
	explicit?: boolean;
	have?: 'in_library' | 'downloading' | 'missing' | '';
}

/** AlbumCoreData is the album hero's quick answer (?part=core). */
export interface AlbumCoreData {
	mbid: string;
	name: string;
	type: string;
	secondary_types: string[];
	release_date?: string;
	disambiguation?: string;
	image: string;
	artist: PageArtist;
	kid: boolean;
}

export interface AlbumPageData extends PageAlbum {
	artist: PageArtist;
	title_id?: number;
	label?: string;
	length_ms: number;
	tracks: PageTrack[];
	editions: { mbid: string; label: string; tracks: number; date?: string }[];
	more_by: PageAlbum[];
	similar: PageArtist[];
	kid: boolean;
	incomplete?: boolean;
}

/** artistHref opens an artist's page. */
export const artistHref = (mbid: string) => to(`/music/artist/${mbid}`);
/** albumHref opens an album's page; the artist id says whose album it is. */
export const albumHref = (mbid: string, artist: string) => to(`/music/album/${mbid}?artist=${artist}`);
/** appPicture puts a server picture path under the app's base path; outside URLs stay. */
export const appPicture = (src: string | undefined | null) => (!src ? null : src.startsWith('/') ? to(src) : src);

/** owned is the card's status pill for an album's Library state. */
export function owned(a: Pick<PageAlbum, 'state'>, asked = false): { state: 'available' | 'partial' | 'downloading' | 'requested'; label: string } | null {
	switch (a.state) {
		case 'complete':
			return { state: 'available', label: 'In your Library' };
		case 'partial':
			return { state: 'partial', label: 'Some tracks in your Library' };
		case 'downloading':
			return { state: 'downloading', label: 'Downloading' };
		case 'missing':
			return { state: 'requested', label: 'Missing: wanted, no copy yet' };
	}
	return asked ? { state: 'requested', label: 'Requested' } : null;
}

/** inLibrary says the Library keeps the album (any state but left out). */
export const kept = (a: Pick<PageAlbum, 'state'>) => !!a.state && a.state !== 'not_included';

export const SHELVES: { shelf: Shelf; title: string }[] = [
	{ shelf: 'albums', title: 'Albums' },
	{ shelf: 'singles', title: 'EPs and singles' },
	{ shelf: 'live', title: 'Live and compilations' },
	{ shelf: 'other', title: 'Soundtracks, remixes and more' }
];

/** shelves splits the out-already albums by shelf (newest first, as served). */
export function shelves(albums: PageAlbum[]): { shelf: Shelf; title: string; albums: PageAlbum[] }[] {
	return SHELVES.map((s) => ({ ...s, albums: albums.filter((a) => a.shelf === s.shelf && !a.upcoming) })).filter((s) => s.albums.length);
}

/** minutes says a length like 3:07, or 1 hr 12 min for a whole album. */
export function length(ms: number | undefined, long = false): string {
	if (!ms) return '';
	const s = Math.round(ms / 1000);
	if (long) {
		const m = Math.round(s / 60);
		return m >= 60 ? `${Math.floor(m / 60)} hr ${m % 60} min` : `${m} min`;
	}
	return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`;
}

/** kindWord names an album's kind for its meta line. */
export function kindWord(a: Pick<PageAlbum, 'type' | 'secondary_types'>): string {
	const sec = a.secondary_types?.[0];
	if (sec) return sec.charAt(0).toUpperCase() + sec.slice(1);
	if (a.type === 'ep') return 'EP';
	return a.type ? a.type.charAt(0).toUpperCase() + a.type.slice(1) : 'Album';
}

/** haveLine is the hero's "On HolyArrs" line. */
export function haveLine(have: number, total: number): string {
	if (!total) return '';
	return `On HolyArrs: ${have} of ${total} ${total === 1 ? 'release' : 'releases'}`;
}
