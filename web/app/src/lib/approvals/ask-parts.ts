// What a music or TV ask is made of, for the Approvals panel's lists: an
// artist ask's albums (one row per album, its other editions folded under
// it, singles and EPs, live albums and compilations counted apart), and a
// show ask's seasons. The approver can leave some out before approving.

import type { Ask } from '../types.ts';
import type { SeasonInfo } from './inbox.ts';

/** An album as the artist answer lists it (a MusicBrainz release group). */
export interface ArtistAlbum {
	mbid: string;
	name: string;
	type: string;
	secondary_types: string[];
	release_date?: string;
	shelf: string;
	image: string;
	upcoming?: boolean;
}

/** The music add defaults the whole-artist ask follows. */
export interface MusicKinds {
	album_types: string[];
	secondary_types: string[];
}
export const musicDefaults: MusicKinds = { album_types: ['album', 'ep'], secondary_types: ['studio'] };

/** The kinds an album is counted under, in this order. */
export type AlbumKind = 'albums' | 'singles' | 'live' | 'compilations' | 'other';
const kindWords: Record<AlbumKind, [string, string]> = {
	albums: ['album', 'albums'],
	singles: ['single or EP', 'singles & EPs'],
	live: ['live album', 'live albums'],
	compilations: ['compilation', 'compilations'],
	other: ['soundtrack or remix', 'soundtracks & remixes']
};
const kindOrder: AlbumKind[] = ['albums', 'singles', 'live', 'compilations', 'other'];

/** One album row: the main edition, with the other editions of the same album under it. */
export interface AlbumGroup {
	key: string;
	kind: AlbumKind;
	main: { mbid: string; name: string; year?: number; image: string };
	/** Every edition's id, the main one first. */
	ids: string[];
	/** The other editions (deluxe, remaster, anniversary…), oldest first. */
	editions: { mbid: string; name: string; year?: number }[];
}

// Words that mark an edition of an album rather than a new album.
const editionWords = /\b(deluxe|remaster(ed)?|anniversary|expanded|edition|version|explicit|clean|bonus|special|super|collector'?s?|reissue|mono|stereo|japan(ese)?|uk|us|international|tour|platinum|complete|legacy|redux|\d{1,2}(st|nd|rd|th))\b/i;

/** baseName is an album's name without an edition tag: "Album (Deluxe Edition)" and "Album - 2011 Remaster" are "album". */
export function baseName(name: string): string {
	let s = name.trim();
	for (;;) {
		const m = s.match(/^(.*?)\s*(?:[([]([^()[\]]*)[)\]]|\s[-–:]\s+([^-–:]+))$/);
		const tag = m ? (m[2] ?? m[3] ?? '') : '';
		if (!m || !m[1] || !editionWords.test(tag)) break;
		s = m[1];
	}
	return s.toLowerCase().replace(/\s+/g, ' ').trim();
}

const yearOf = (d: string | undefined) => (d && /^\d{4}/.test(d) ? Number(d.slice(0, 4)) : undefined);

// Live recordings MusicBrainz lists without the live mark: "Live at Lowlands", "Night (Live)".
const liveName = /^live\s+(at|in|from|on)\b|[([]live[^)\]]*[)\]]/i;

/** kindOf sorts an album into a counted kind, as the artist page's shelves do (a live name counts as live). */
export function kindOf(al: Pick<ArtistAlbum, 'type' | 'secondary_types' | 'shelf'> & { name?: string }): AlbumKind {
	if (al.name && liveName.test(al.name) && (al.shelf === 'albums' || al.shelf === 'singles' || !al.shelf)) return 'live';
	if (al.shelf === 'albums' || al.shelf === 'singles' || al.shelf === 'live' || al.shelf === 'compilations') return al.shelf;
	if (al.shelf) return 'other';
	const sec = (al.secondary_types ?? []).map((x) => x.toLowerCase());
	if (sec.includes('live')) return 'live';
	if (sec.includes('compilation')) return 'compilations';
	if (sec.length) return 'other';
	return al.type === 'album' ? 'albums' : al.type === 'ep' || al.type === 'single' ? 'singles' : 'other';
}

/** keptByDefault says whether the whole-artist ask takes this album (the music add defaults). */
export function keptByDefault(al: Pick<ArtistAlbum, 'type' | 'secondary_types' | 'upcoming'>, k: MusicKinds = musicDefaults): boolean {
	if (al.upcoming) return false;
	if (!k.album_types.includes(al.type)) return false;
	const sec = al.secondary_types?.length ? al.secondary_types : ['studio'];
	return sec.every((s) => k.secondary_types.includes(s));
}

/**
 * albumGroups folds an artist ask's albums into rows: editions of one album
 * (same name once the edition tag is dropped, same kind) share a row under
 * the oldest plain edition. Rows run by kind, then oldest first.
 */
export function albumGroups(list: Pick<ArtistAlbum, 'mbid' | 'name' | 'type' | 'secondary_types' | 'release_date' | 'shelf' | 'image'>[]): AlbumGroup[] {
	const by = new Map<string, (typeof list)[number][]>();
	for (const al of list) {
		const key = `${kindOf(al)}|${baseName(al.name)}`;
		const g = by.get(key) ?? [];
		g.push(al);
		by.set(key, g);
	}
	const out: AlbumGroup[] = [];
	for (const [key, g] of by) {
		const sorted = [...g].sort((x, y) => (x.release_date ?? '9999').localeCompare(y.release_date ?? '9999') || x.name.length - y.name.length);
		// The main edition: the oldest one without an edition tag, else the oldest.
		const main = sorted.find((x) => baseName(x.name) === x.name.trim().toLowerCase().replace(/\s+/g, ' ')) ?? sorted[0];
		const rest = sorted.filter((x) => x !== main);
		out.push({
			key,
			kind: kindOf(main),
			main: { mbid: main.mbid, name: main.name, year: yearOf(main.release_date), image: main.image },
			ids: [main.mbid, ...rest.map((x) => x.mbid)],
			editions: rest.map((x) => ({ mbid: x.mbid, name: x.name, year: yearOf(x.release_date) }))
		});
	}
	return out.sort((x, y) => kindOrder.indexOf(x.kind) - kindOrder.indexOf(y.kind) || (x.main.year ?? 9999) - (y.main.year ?? 9999) || x.main.name.localeCompare(y.main.name));
}

/** albumCounts says "12 albums · 8 singles & EPs" over the rows (editions counted once). */
export function albumCounts(groups: Pick<AlbumGroup, 'kind'>[]): string {
	const n = new Map<AlbumKind, number>();
	for (const g of groups) n.set(g.kind, (n.get(g.kind) ?? 0) + 1);
	return kindOrder
		.filter((k) => n.get(k))
		.map((k) => `${n.get(k)} ${kindWords[k][n.get(k) === 1 ? 0 : 1]}`)
		.join(' · ');
}

/**
 * askAlbums is what an artist ask is for, as album rows: the albums it
 * names, or (a whole-artist ask) the artist's released albums the music
 * defaults take. Albums the artist answer does not list keep their name.
 */
export function askAlbums(a: Pick<Ask, 'albums'>, artist: ArtistAlbum[] | null, kinds: MusicKinds = musicDefaults, cover: (mbid: string) => string = () => ''): ArtistAlbum[] {
	if (a.albums?.length) {
		return a.albums.map((w) => {
			const k = artist?.find((x) => x.mbid === w.mbid);
			return k ?? { mbid: w.mbid, name: w.name, type: 'album', secondary_types: [], release_date: w.year ? String(w.year) : undefined, shelf: 'albums', image: cover(w.mbid) };
		});
	}
	return (artist ?? []).filter((x) => keptByDefault(x, kinds));
}

/** A season row of a show ask. */
export interface SeasonRow {
	number: number;
	episodes: number;
	year?: number;
}

/** askSeasons lists the seasons a show ask is for: the ones it names, or every regular season of a whole-show ask. */
export function askSeasons(a: Pick<Ask, 'seasons' | 'future_only'>, seasons: SeasonInfo[]): SeasonRow[] {
	if (a.future_only) return [];
	const regular = seasons.filter((s) => s.number > 0);
	const want = a.seasons?.length ? a.seasons : regular.map((s) => s.number);
	return [...new Set(want)]
		.sort((x, y) => x - y)
		.map((n) => {
			const s = regular.find((x) => x.number === n);
			return { number: n, episodes: s?.episode_count ?? 0, year: s?.year };
		});
}

/** keptWords says "3 of 4 seasons · 30 episodes" or "11 of 12 albums" after the approver left some out. */
export function keptWords(kept: number, all: number, noun: string, extra = ''): string {
	const many = all === 1 ? noun : `${noun}s`;
	const head = kept === all ? (all === 1 ? `1 ${noun}` : `All ${all} ${many}`) : `${kept} of ${all} ${many}`;
	return extra ? `${head} · ${extra}` : head;
}

/** What the approver keeps of a music or TV ask before approving. */
export interface PartsPick {
	/** Seasons kept (show asks), null when all are. */
	seasons: number[] | null;
	/** Album ids kept, every edition of each kept album (artist asks), null when all are. */
	albums: string[] | null;
	/** Everything was unticked: nothing to approve. */
	empty: boolean;
}
