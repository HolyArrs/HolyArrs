// What a music or TV ask is made of, for the Requests panel's lists: an artist ask's albums (one row per album, its other editions folded under it, singles and EPs, live albums and compilations counted apart), and a show ask's seasons.

import { t } from '#lib/core/i18n/i18n.svelte.ts';
import type { SeasonInfo } from './inbox.ts';
import type { Ask } from '../core/types.ts';

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
	/** The album's place in the Library (its stored tracks), when the artist is in it. */
	part_id?: number;
}

/** The music add defaults the whole-artist ask follows. */
export interface MusicKinds {
	album_types: string[];
	secondary_types: string[];
}

/** The kinds an album is counted under, in this order. */
export type AlbumKind = 'albums' | 'singles' | 'live' | 'compilations' | 'other';

/** One album row: the main edition, with the other editions of the same album under it. */
export interface AlbumGroup {
	key: string;
	kind: AlbumKind;
	main: { mbid: string; name: string; year?: number; image: string; part_id?: number };
	/** Every edition's id, the main one first. */
	ids: string[];
	/** The other editions (deluxe, remaster, anniversary), oldest first. */
	editions: { mbid: string; name: string; year?: number }[];
}

/** A season row of a show ask. */
export interface SeasonRow {
	number: number;
	episodes: number;
	year?: number;
	/** The single episodes asked for; absent means the whole season. */
	asked?: number[];
}

/** The tick state of a season: every episode, some, or none. */
export type SeasonTick = 'all' | 'some' | 'none';

/** One season as the approver sees it: its episodes, the ones asked for, and the ones ticked. */
export interface SeasonPick {
	number: number;
	/** Every episode number of the season (from the list, else 1 to count). */
	all: number[];
	/** The episodes the person asked for. */
	asked: number[];
	/** The episodes that will be added (ticked). */
	ticked: number[];
	tick: SeasonTick;
}

/** What the approver keeps of a music or TV ask before approving. */
export interface PartsPick {
	/** Seasons kept (show asks); null when all are. */
	seasons: number[] | null;
	/** Single episodes kept in partly ticked seasons (show asks); null when none. */
	episodes?: { season: number; episodes: number[] }[] | null;
	/** Songs kept in partly ticked albums (recording ids); null when none. */
	songs?: { album: string; recordings: string[] }[] | null;
	/** Album ids kept, every edition of each kept album (artist asks); null when all are. */
	albums: string[] | null;
	/** Everything was unticked: nothing to approve. */
	empty: boolean;
}

export const musicDefaults: MusicKinds = { album_types: ['album', 'ep'], secondary_types: ['studio'] };

const kindOrder: AlbumKind[] = ['albums', 'singles', 'live', 'compilations', 'other'];

// Words that mark an edition of an album rather than a new album.
const editionWords = /\b(deluxe|remaster(ed)?|anniversary|expanded|edition|version|explicit|clean|bonus|special|super|collector'?s?|reissue|mono|stereo|japan(ese)?|uk|us|international|tour|platinum|complete|legacy|redux|\d{1,2}(st|nd|rd|th))\b/i;

/** An album's name without an edition tag: "Album (Deluxe Edition)" and "Album - 2011 Remaster" are "album". */
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

// Title words that mark a release as not a real album (MusicBrainz sometimes leaves these untyped).
export const notAlbumName = /\b(demos?|bootlegs?|outtakes?|unreleased|rehearsals?|b-sides|leaked|snippets?|remix(es)?|remixed)\b/i;

/** Sorts an album into a counted kind, as the artist page's shelves do (a live name counts as live). */
export function kindOf(al: Pick<ArtistAlbum, 'type' | 'secondary_types' | 'shelf'> & { name?: string }): AlbumKind {
	if (al.name && liveName.test(al.name) && (al.shelf === 'albums' || al.shelf === 'singles' || !al.shelf)) return 'live';
	if (al.name && notAlbumName.test(al.name) && (al.shelf === 'albums' || !al.shelf)) return 'other';
	if (al.shelf === 'albums' || al.shelf === 'singles' || al.shelf === 'live' || al.shelf === 'compilations') return al.shelf;
	if (al.shelf) return 'other';
	const sec = (al.secondary_types ?? []).map((x) => x.toLowerCase());
	if (sec.includes('live')) return 'live';
	if (sec.includes('compilation')) return 'compilations';
	if (sec.length) return 'other';
	return al.type === 'album' ? 'albums' : al.type === 'ep' || al.type === 'single' ? 'singles' : 'other';
}

/** Whether the whole-artist ask takes this album (the music add defaults). */
export function keptByDefault(al: Pick<ArtistAlbum, 'type' | 'secondary_types' | 'upcoming'>, k: MusicKinds = musicDefaults): boolean {
	if (al.upcoming) return false;
	if (!k.album_types.includes(al.type)) return false;
	const sec = al.secondary_types?.length ? al.secondary_types : ['studio'];
	return sec.every((s) => k.secondary_types.includes(s));
}

/** Folds an artist ask's albums into rows. Editions of one album (same name once the edition tag is dropped, same kind) share a row under the oldest plain edition. Rows run by kind, then oldest first. */
export function albumGroups(list: (Pick<ArtistAlbum, 'mbid' | 'name' | 'type' | 'secondary_types' | 'release_date' | 'shelf' | 'image'> & { part_id?: number })[]): AlbumGroup[] {
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
			main: { mbid: main.mbid, name: main.name, year: yearOf(main.release_date), image: main.image, ...(main.part_id ? { part_id: main.part_id } : {}) },
			ids: [main.mbid, ...rest.map((x) => x.mbid)],
			editions: rest.map((x) => ({ mbid: x.mbid, name: x.name, year: yearOf(x.release_date) }))
		});
	}
	return out.sort((x, y) => kindOrder.indexOf(x.kind) - kindOrder.indexOf(y.kind) || (x.main.year ?? 9999) - (y.main.year ?? 9999) || x.main.name.localeCompare(y.main.name));
}

/** "12 albums · 8 singles & EPs" over the rows (editions counted once). */
export function albumCounts(groups: Pick<AlbumGroup, 'kind'>[]): string {
	const n = new Map<AlbumKind, number>();
	for (const g of groups) n.set(g.kind, (n.get(g.kind) ?? 0) + 1);
	return kindOrder
		.filter((k) => n.get(k))
		.map((k) => t(`requests.albumKind.${k}.${n.get(k) === 1 ? 'one' : 'other'}`, { n: n.get(k) ?? 0 }))
		.join(' · ');
}

/** What an artist ask is for, as album rows: the albums it names, or (a whole-artist ask) the artist's released albums the music defaults take. Albums the artist answer does not list keep their name. */
export function askAlbums(a: Pick<Ask, 'albums'>, artist: ArtistAlbum[] | null, kinds: MusicKinds = musicDefaults, cover: (mbid: string) => string = () => ''): ArtistAlbum[] {
	if (a.albums?.length) {
		return a.albums.map((w) => {
			const k = artist?.find((x) => x.mbid === w.mbid);
			return k ?? { mbid: w.mbid, name: w.name, type: 'album', secondary_types: [], release_date: w.year ? String(w.year) : undefined, shelf: 'albums', image: cover(w.mbid) };
		});
	}
	return (artist ?? []).filter((x) => keptByDefault(x, kinds));
}

/** The seasons a show ask is for: the ones it names (whole, or with single episodes), or every regular season of a whole-show ask. */
export function askSeasons(a: Pick<Ask, 'seasons' | 'future_only'> & Pick<Partial<Ask>, 'episodes'>, seasons: SeasonInfo[]): SeasonRow[] {
	if (a.future_only) return [];
	const regular = seasons.filter((s) => s.number > 0);
	const single = (a.episodes ?? []).filter((e) => e.episodes?.length);
	const want = a.seasons?.length || single.length ? [...(a.seasons ?? []), ...single.map((e) => e.season)] : regular.map((s) => s.number);
	return [...new Set(want)]
		.sort((x, y) => x - y)
		.map((n) => {
			const s = regular.find((x) => x.number === n);
			const asked = a.seasons?.includes(n) ? undefined : single.find((e) => e.season === n)?.episodes;
			return { number: n, episodes: s?.episode_count ?? 0, year: s?.year, ...(asked ? { asked: [...new Set(asked)].sort((x, y) => x - y) } : {}) };
		});
}

const range = (n: number) => Array.from({ length: n }, (_, i) => i + 1);

/** Works out one season's ticks; `ticked` is the approver's own pick (undefined means as asked). */
export function seasonPick(row: SeasonRow, list: { number: number }[] | null, ticked?: number[]): SeasonPick {
	const all = list?.length ? list.map((e) => e.number) : range(row.episodes);
	const asked = row.asked ? row.asked : all;
	const t = (ticked ?? asked).filter((n) => !all.length || all.includes(n));
	// A season with no episode list yet is ticked whole unless the approver unticked it.
	const tick: SeasonTick = !all.length ? (ticked && !ticked.length ? 'none' : 'all') : !t.length ? 'none' : t.length < all.length ? 'some' : 'all';
	return { number: row.number, all, asked, ticked: t, tick };
}

const episodes = (n: number) => t(`requests.parts.episodes.${n === 1 ? 'one' : 'other'}`, { n });
const seasons = (n: number) => t(`requests.parts.seasons.${n === 1 ? 'one' : 'other'}`, { n });

/** What the person asked of a season: "all 8 episodes", "5 of 8 episodes". */
export function askedWords(p: Pick<SeasonPick, 'all' | 'asked'>): string {
	const n = p.all.length;
	if (!n) return p.asked.length ? episodes(p.asked.length) : t('requests.parts.notListed');
	if (p.asked.length >= n) return n === 1 ? episodes(1) : t('requests.parts.allEpisodes', { n });
	return t('requests.parts.someEpisodes', { kept: p.asked.length, n });
}

/** Sums the ticks: "All 3 seasons · 16 episodes", "2 of 3 seasons · 18 episodes", "2 seasons + 5 episodes · 21 episodes". */
export function picksHead(picks: SeasonPick[]): string {
	const whole = picks.filter((p) => p.tick === 'all');
	const some = picks.filter((p) => p.tick === 'some');
	const eps = picks.reduce((n, p) => n + p.ticked.length, 0);
	const total = eps ? episodes(eps) : '';
	if (!some.length) return keptWords(whole.length, picks.length, 'season', total);
	const single = some.reduce((n, p) => n + p.ticked.length, 0);
	const head = [whole.length ? seasons(whole.length) : '', episodes(single)].filter(Boolean).join(' + ');
	return whole.length ? `${head} · ${total}` : some.length === 1 ? t('requests.parts.fromSeason', { head, n: some[0].number }) : t('requests.parts.fromSeasons', { head, n: some.length });
}

/** Whether the ticks differ from what was asked. */
export const picksChanged = (picks: SeasonPick[]) => picks.some((p) => p.ticked.length !== p.asked.length || p.ticked.some((n) => !p.asked.includes(n)));

/** "3 of 4 seasons · 30 episodes" or "11 of 12 albums" after the approver left some out. */
export function keptWords(kept: number, all: number, noun: 'season' | 'album', extra = ''): string {
	const head = kept === all ? (all === 1 ? t(`requests.kept.${noun}.one`) : t(`requests.kept.${noun}.all`, { n: all })) : t(`requests.kept.${noun}.${all === 1 ? 'someOne' : 'some'}`, { kept, all });
	return extra ? `${head} · ${extra}` : head;
}

/** Adds the song total to the album counts: "... · 112 songs", "... · 98 of 112 songs" (while lists load: the songs known so far). */
export function songsHead(albums: string, rows: { all: number | null; ticked: number }[]): string {
	const known = rows.filter((r) => r.all !== null && r.all > 0);
	if (!known.length) return albums;
	const all = known.reduce((n, r) => n + (r.all ?? 0), 0);
	const kept = known.reduce((n, r) => n + r.ticked, 0);
	const words = kept === all ? t(`requests.parts.songs.${all === 1 ? 'one' : 'other'}`, { n: all }) : t('requests.parts.someSongs', { kept, all });
	return albums ? `${albums} · ${words}` : words;
}
