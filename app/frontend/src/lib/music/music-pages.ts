// The artist and album pages: types, links and the small rules they share.
import { to } from '../core/paths.ts';
import { appPicture, coverFor, coverSize } from '../core/pictures.ts';
import { t, lang, appLocale } from '#lib/core/i18n/i18n.svelte.ts';

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

/** The hero's quick answer (?part=core); the full answer extends it. */
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
	/** The stored complete discography, so the shelves draw with the hero (no Library state yet). */
	albums?: PageAlbum[];
}

export interface ArtistPopularity {
	monthly_listeners?: number;
	world_rank?: number;
	listeners_change_today?: number;
	total_plays?: number;
	as_of?: string;
}

export interface StoreAlbum {
	title: string;
	release_date?: string;
	type: string;
	track_count?: number;
	explicit?: boolean;
	image?: string;
}

/** The one headline figure an artist has: monthly listeners (with rank and today's change) for the most played, else total plays; null when none is known. */
export interface Listening {
	n: number;
	/** Already in the person's language. */
	unit: string;
	rank?: number;
	today?: number;
}

/** What an artist has put out, for the stats band. */
export interface Catalog {
	albums: number;
	eps: number;
	singles: number;
	/** First release year, e.g. "2005". */
	since: string;
	/** First and last year joined by an en dash, or one year when all came out the same year. */
	span: string;
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
	/** Listening figures: listeners, rank and today's change for the most played artists, else total plays; absent when none is known. */
	popularity?: ArtistPopularity;
	/** The releases a store knows when MusicBrainz lists none: shown, never asked for. */
	store_albums?: StoreAlbum[];
	similar: PageArtist[];
	related: PageArtist[];
	kid: boolean;
	/** What "Ask for all missing" asks: released items the Library does not keep. */
	missing?: { count: number; size_bytes: number; albums: string[] };
	incomplete?: boolean;
	/** The server is still getting this artist (or it waits in line): its words show in place of the content. */
	gap?: PageGap;
}

/** The metadata server's word on an item it is still getting. */
export interface PageGap {
	state: 'fetching' | 'queued';
	position?: number;
	message: string;
}

/** A release on another artist's page or a neighbour's top album. */
export interface PageCredit extends PageAlbum {
	artist_mbid: string;
	artist_name: string;
	listeners?: number;
}

/** Answers ?part=more: Essential albums (ids) and Appears on. */
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

/** The album hero's quick answer (?part=core). */
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

/** One edition of an album, with its own track list. */
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
	gap?: PageGap;
}

export { appPicture, coverFor, coverSize };

/** Asks once more after a pause when the first ask fails, so a slow first load never shows an error. */
export function askAgainOnce<T>(ask: () => Promise<T>, pause = 1500, wait: (ms: number) => Promise<void> = (ms) => new Promise((r) => setTimeout(r, ms))): Promise<T> {
	return ask().catch(async () => {
		await wait(pause);
		return ask();
	});
}

/** Writes 1234 as 1.2K and 81500000 as 81.5M. */
export function shortCount(n: number): string {
	const a = Math.abs(n);
	const f = (v: number, u: string) => `${v >= 100 ? Math.round(v) : Number(v.toFixed(1))}${u}`;
	if (a >= 1e9) return f(a / 1e9, 'B');
	if (a >= 1e6) return f(a / 1e6, 'M');
	if (a >= 1e3) return f(a / 1e3, 'K');
	return String(Math.round(a));
}

export function listening(p: ArtistPopularity | undefined | null): Listening | null {
	if (!p) return null;
	if (p.monthly_listeners && p.monthly_listeners > 0) {
		const out: Listening = { n: p.monthly_listeners, unit: t('music.monthlyListeners') };
		if (p.world_rank && p.world_rank > 0) out.rank = p.world_rank;
		if (p.listeners_change_today) out.today = p.listeners_change_today;
		return out;
	}
	if (p.total_plays && p.total_plays > 0) return { n: p.total_plays, unit: t('music.totalPlays') };
	return null;
}

/** Today's listener change as "▲ 387K today" or "▼ 12K today"; empty when there is none. */
export function changeWords(d: number | undefined | null, tail?: string): string {
	if (!d) return '';
	return `${d > 0 ? '▲' : '▼'} ${shortCount(d)}${tail ?? ' ' + t('music.today')}`;
}

/** Writes 72785924 as 72,785,924. */
export function fullCount(n: number): string {
	return Math.round(n).toLocaleString(appLocale('en-US'));
}

const plural = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`;

/** Turns a failed music page load into calm words. `network` is true only when HolyArrs itself could not be reached, the one case where "check the connection" is true. */
export function musicLoadError(e: unknown, thing: 'album' | 'artist'): { text: string; network: boolean } {
	const status = typeof e === 'object' && e && 'status' in e ? Number((e as { status: unknown }).status) : -1;
	if (status === 0) return { text: t('music.holyarrsIsNotAnswering'), network: true };
	if (status === 404) return { text: thing === 'album' ? t('music.weCouldNotFind') : t('music.weCouldNotFind2'), network: false };
	if (status === 400) return { text: thing === 'album' ? t('music.thatAlbumLinkIs') : t('music.thatArtistLinkIs'), network: false };
	if (status === 401 || status === 403) return { text: thing === 'album' ? t('music.youCannotOpenThis') : t('music.youCannotOpenThis2'), network: false };
	return { text: thing === 'album' ? t('music.thisAlbumIsTaking') : t('music.thisArtistIsTaking'), network: false };
}

/** The artist to put in the address when the album named a different one than the link did, else ''. */
export function albumArtistFix(linkArtist: string, found: string | undefined): string {
	const real = (found ?? '').toLowerCase();
	return real && real !== linkArtist.toLowerCase() ? real : '';
}

/** Opens an artist's page. */
export const artistHref = (mbid: string) => to(`/music/artist/${mbid}`);

/** Opens an album's page; the artist id says whose album it is. */
export const albumHref = (mbid: string, artist: string) => to(`/music/album/${mbid}?artist=${artist}`);

/** Keeps the albums and artists that have a real picture, so a row never shows a letter tile. */
export const withCover = <T extends { image?: string | null }>(list: readonly T[] | null | undefined): T[] => (list ?? []).filter((x) => !!x.image?.trim());

/** The card's status pill for an album's Library state. */
export function owned(a: Pick<PageAlbum, 'state'>, asked = false): { state: 'available' | 'partial' | 'downloading' | 'requested'; label: string } | null {
	switch (a.state) {
		case 'complete':
			return { state: 'available', label: t('music.inYourLibrary') };
		case 'partial':
			return { state: 'partial', label: t('music.someTracksInYour') };
		case 'downloading':
			return { state: 'downloading', label: t('music.downloading') };
		case 'missing':
			return { state: 'requested', label: t('music.missingNoCopy') };
	}
	return asked ? { state: 'requested', label: t('music.requested') } : null;
}

/** Whether the Library keeps the album (any state but left out). */
export const kept = (a: Pick<PageAlbum, 'state'>) => !!a.state && a.state !== 'not_included';

export const SHELVES: { shelf: Shelf; title: string }[] = [
	{ shelf: 'albums', get title() { return t('music.albums'); } },
	{ shelf: 'singles', get title() { return t('music.epsAndSingles'); } },
	{ shelf: 'compilations', get title() { return t('music.compilations'); } },
	{ shelf: 'live', get title() { return t('music.live'); } },
	{ shelf: 'other', get title() { return t('music.soundtracksRemixesAndMore'); } }
];

/** Splits the out-already albums by shelf (newest first, as served). */
export function shelves(albums: PageAlbum[]): { shelf: Shelf; title: string; albums: PageAlbum[] }[] {
	return SHELVES.map((s) => ({ ...s, albums: albums.filter((a) => a.shelf === s.shelf && !a.upcoming) })).filter((s) => s.albums.length);
}

/** A length like 3:07, or 1 hr 12 min for a whole album. */
export function length(ms: number | undefined, long = false): string {
	if (!ms) return '';
	const s = Math.round(ms / 1000);
	if (long) {
		const m = Math.round(s / 60);
		return m >= 60 ? t('music.hrMin', { h: Math.floor(m / 60), m: m % 60 }) : t('music.min', { m });
	}
	return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`;
}

/** An album's kind for its meta line. */
export function kindWord(a: Pick<PageAlbum, 'type' | 'secondary_types'>): string {
	const sec = a.secondary_types?.[0];
	if (sec) return sec.charAt(0).toUpperCase() + sec.slice(1);
	if (a.type === 'ep') return t('music.ep');
	return a.type ? a.type.charAt(0).toUpperCase() + a.type.slice(1) : t('music.album');
}

/** A byte count as "about 1.2 GB" for the confirm line. */
export function aboutSize(bytes: number): string {
	if (bytes >= 1 << 30) return t('music.aboutGb', { n: (bytes / (1 << 30)).toFixed(1) });
	return t('music.aboutMb', { n: Math.max(1, Math.round(bytes / (1 << 20))) });
}

/** What kind of act an artist is, in our words: a person is a solo artist; a group, orchestra or choir is a group. */
export function artistKind(type?: string): string {
	switch ((type ?? '').toLowerCase()) {
		case 'person':
			return t('music.soloArtist');
		case 'group':
		case 'orchestra':
		case 'choir':
			return t('music.group');
		case '':
			return '';
		default:
			return t('music.artist');
	}
}

/** Spells out a two-letter country code (CA: Canada); other codes stay as they are. */
export function countryName(code?: string): string {
	const c = (code ?? '').trim();
	if (!/^[A-Za-z]{2}$/.test(c)) return c;
	try {
		return new Intl.DisplayNames([lang.value === 'pt-br' ? 'pt-BR' : lang.value === 'zh-cn' ? 'zh-CN' : lang.value], { type: 'region' }).of(c.toUpperCase()) ?? c;
	} catch {
		return c;
	}
}

/** How long the full page answer gets before the quick core part is asked too. */
export const CORE_AFTER_MS = 250;

/** Asks the quick core part only when the full answer (which holds the core) has not come within `after` ms, so a fast or kept page costs one request. */
export function coreIfSlow<T>(full: Promise<unknown>, askCore: () => Promise<T>, after = CORE_AFTER_MS, wait: (ms: number) => Promise<void> = (ms) => new Promise((r) => setTimeout(r, ms))): Promise<T | null> {
	let done = false;
	const settle = () => (done = true);
	full.then(settle, settle);
	return wait(after).then(() => (done ? null : askCore()));
}
