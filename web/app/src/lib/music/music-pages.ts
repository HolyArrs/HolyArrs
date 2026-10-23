// The artist and album pages: types, links and the small rules they share.
import { to } from '../paths.ts';

export type PageAlbumState = 'complete' | 'partial' | 'missing' | 'downloading' | 'not_out_yet' | 'not_included' | '';
export type Shelf = 'albums' | 'singles' | 'compilations' | 'live' | 'other';

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
	/** What "Ask for all missing" asks: released items the Library does not keep. */
	missing?: { count: number; size_bytes: number; albums: string[] };
}

/** PageCredit is a release on another artist's page or a neighbour's top album. */
export interface PageCredit extends PageAlbum {
	artist_mbid: string;
	artist_name: string;
	listeners?: number;
}

/** ArtistMoreData answers ?part=more: Essential albums (ids) and Appears on. */
export interface ArtistMoreData {
	essential: string[];
	appears_on: PageCredit[];
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

/** PageEdition is one edition of an album, with its own track list. */
export interface PageEdition {
	mbid: string;
	label: string;
	tracks: number;
	date?: string;
	list?: PageTrack[];
}

export interface AlbumPageData extends PageAlbum {
	artist: PageArtist;
	title_id?: number;
	label?: string;
	length_ms: number;
	tracks: PageTrack[];
	editions: PageEdition[];
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
import { versioned } from './photo-version.ts';
export const appPicture = (src: string | undefined | null) => (!src ? null : src.startsWith('/') ? to(versioned(src)) : src);

const caa = /^(https?:\/\/(?:www\.)?coverartarchive\.org\/release(?:-group)?\/[^/]+\/(?:front|back|\d+))(?:-(?:250|500|1200))?(?:\.jpg)?$/;
const proxied = /\/api\/v1\/(?:music-trending\/art\/album\/[^/?]+|albums\/\d+\/cover)(?:\?.*)?$/;

const deezer = /^(https?:\/\/[a-z0-9-]+\.dzcdn\.net\/images\/(?:cover|artist)\/[0-9a-f]+\/)\d+x\d+(-.*)?$/;
const itunes = /^(https?:\/\/[a-z0-9-]+\.mzstatic\.com\/image\/thumb\/.+\/)\d+x\d+(bb|cc)?(\.(?:jpg|png|webp))$/;

/** coverAt points a sized cover link (Cover Art Archive, Deezer, iTunes, our
 * cover proxies) at s px, or null when the link has no size form. */
function coverAt(src: string): ((s: number) => string) | null {
	let m = caa.exec(src);
	if (m) return (s) => `${m![1]}-${s}`;
	if ((m = deezer.exec(src))) return (s) => `${m![1]}${s}x${s}${m![2] ?? ''}`;
	if ((m = itunes.exec(src))) return (s) => `${m![1]}${s}x${s}${m![2] || 'bb'}${m![3]}`;
	// Our cover proxies take ?size= (the server picks that size and caches each).
	if (proxied.test(src)) return (s) => `${src}${src.includes('?') ? '&' : '?'}size=${s}`;
	return null;
}

/** coverFor sizes a cover for a tile css pixels wide: the smallest of 250,
 * 500 and 1200 that is sharp at this pixel ratio, with a srcset of all three
 * (the card's measured width is its sizes). Other pictures stay as they are. */
export function coverFor(src: string | null | undefined, css: number, ratio = globalThis.devicePixelRatio || 1): { src: string | null; srcset?: string } {
	const px = css * ratio;
	const size = px <= 250 ? 250 : px <= 500 ? 500 : 1200;
	const at = src ? coverAt(src) : null;
	if (!at) return { src: src ?? null };
	return { src: at(size), srcset: [250, 500, 1200].map((s) => `${at(s)} ${s}w`).join(', ') };
}

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
	{ shelf: 'compilations', title: 'Compilations' },
	{ shelf: 'live', title: 'Live' },
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

/** aboutSize writes a byte count as "about 1.2 GB" for the confirm line. */
export function aboutSize(bytes: number): string {
	if (bytes >= 1 << 30) return `about ${(bytes / (1 << 30)).toFixed(1)} GB`;
	return `about ${Math.max(1, Math.round(bytes / (1 << 20)))} MB`;
}

/** artistKind says what kind of act an artist is, in our words: a person is a solo artist; a group, orchestra or choir is a group. */
export function artistKind(type?: string): string {
	switch ((type ?? '').toLowerCase()) {
		case 'person':
			return 'Solo artist';
		case 'group':
		case 'orchestra':
		case 'choir':
			return 'Group';
		case '':
			return '';
		default:
			return 'Artist';
	}
}

/** countryName spells out a two-letter country code (CA: Canada); other codes stay as they are. */
export function countryName(code?: string): string {
	const c = (code ?? '').trim();
	if (!/^[A-Za-z]{2}$/.test(c)) return c;
	try {
		return new Intl.DisplayNames(['en'], { type: 'region' }).of(c.toUpperCase()) ?? c;
	} catch {
		return c;
	}
}

/** initialsOf is up to two initials of a name, for a round photo with no picture. */
export function initialsOf(name?: string): string {
	const words = (name ?? '').split(/\s+/).map((w) => w.replace(/[^\p{L}\p{N}]/gu, '')).filter(Boolean);
	return words.slice(0, 2).map((w) => [...w][0].toUpperCase()).join('');
}
