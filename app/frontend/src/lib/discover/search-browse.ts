import type { ExploreType } from './explore-type.ts';
import type { MusicCardData, MusicExploreAnswer } from '../music/index.ts';

/** One tile of Search's Browse grid (GET /explore/browse): a genre, or one of its mood rows. */
export interface BrowseTile {
	kind: 'movie' | 'show';
	genre: number;
	genre_name: string;
	/** Set on mood tiles: the genre page's mood row title. */
	mood?: string;
	name: string;
	backdrop: string;
	from: string;
	/** The genre page holds movies and shows (type all, the movie genre's id). */
	both?: boolean;
	/** Set on category tiles (GET /explore/browse/section): the category id, its section and title count. */
	category?: string;
	section?: string;
	count?: number;
}

/** One heading of the Browse grid. */
export interface BrowseSection {
	id: string;
	title: string;
}

export interface BrowseAnswer {
	type: ExploreType;
	tiles: BrowseTile[];
	sections?: BrowseSection[];
}

export interface BrowseSectionAnswer {
	type: ExploreType;
	section: string;
	tiles: BrowseTile[];
	/** The server is still building the section: show ghost tiles and ask again. */
	building?: boolean;
}

export interface CategoryView {
	id: string;
	type: 'all' | 'movies' | 'shows';
}

/** A plain Browse tile (People, Music): a picture, a name and where it opens. */
export interface PlainTile {
	key: string;
	name: string;
	href: string;
	image: string;
}

export interface MoodView {
	mood: string;
	genre: number;
	type: 'all' | 'movies' | 'shows';
}

/** A section first shows this many rows, then this many more each time its end nears the viewport. */
export const BROWSE_ROWS = 2;

/** How many of a section's n tiles show with `rows` rows of `cols` (never fewer than one row's worth). */
export function shownCount(n: number, cols: number, rows: number): number {
	return Math.min(n, Math.max(1, cols) * Math.max(1, rows));
}

export function browseSectionPath(t: ExploreType, section: string): string {
	return `/explore/browse/section?${new URLSearchParams({ type: t, section }).toString()}`;
}

export function browseCategoryPath(t: ExploreType, id: string): string {
	return `/explore/browse/category?${new URLSearchParams({ type: t, id }).toString()}`;
}

/** Reads Search's category view from the address (?cat=&type=); null when it is not one. */
export function readCategory(p: Pick<URLSearchParams, 'get'>): CategoryView | null {
	const id = p.get('cat')?.trim();
	const type = p.get('type');
	if (!id || !/^[a-z0-9-]{1,40}$/.test(id) || (type !== 'all' && type !== 'movies' && type !== 'shows')) return null;
	return { id, type };
}

export function browsePath(t: ExploreType): string {
	return `/explore/browse?${new URLSearchParams({ type: t }).toString()}`;
}

/** The Browse grid's tab for a search scope: Movies and Shows only, both under any other scope. */
export function browseType(scope: string): ExploreType {
	return scope === 'movie' ? 'movies' : scope === 'show' ? 'shows' : 'all';
}

/** Under Movies only movie tiles, under TV shows only TV ones; All keeps every (merged) tile. */
export function strictTiles(tiles: BrowseTile[], tab: ExploreType): BrowseTile[] {
	if (tab === 'movies') return tiles.filter((t) => t.kind === 'movie');
	if (tab === 'shows') return tiles.filter((t) => t.kind === 'show');
	return tiles;
}

/** One tile per music genre the Explore Music answer names, its picture an album cover of that genre (each cover once). Artist pictures never stand in for a cover. */
export function musicTiles(a: Pick<MusicExploreAnswer, 'genres' | 'rows' | 'hero'>, open: (genre: string) => string, pic: (c: MusicCardData) => string | null): PlainTile[] {
	const cards = [...(a.hero ? [a.hero] : []), ...a.rows.flatMap((r) => r.items)];
	const used = new Set<string>();
	const out: PlainTile[] = [];
	for (const g of a.genres ?? []) {
		const hit = cards.find((c) => {
			const p = pic(c);
			return c.kind === 'album' && !!p && !used.has(p) && (c.genres ?? []).some((x) => x.toLowerCase() === g.toLowerCase());
		});
		const image = hit ? pic(hit) : null;
		if (!image) continue;
		used.add(image);
		out.push({ key: `music:${g}`, name: g.replace(/\b\w/g, (m) => m.toUpperCase()), href: open(g), image });
	}
	return out;
}

const tabOf = (t: Pick<BrowseTile, 'kind' | 'both'>): 'all' | 'movies' | 'shows' => (t.both ? 'all' : t.kind === 'show' ? 'shows' : 'movies');

/** A genre tile opens the Explore genre page, a mood tile Search's mood view. */
export function browseHref(t: Pick<BrowseTile, 'kind' | 'genre' | 'mood' | 'both' | 'category'>, tab?: ExploreType): string {
	if (t.category) return `/search?${new URLSearchParams({ cat: t.category, type: tab === 'movies' || tab === 'shows' || tab === 'all' ? tab : t.kind === 'show' ? 'shows' : 'movies' }).toString()}`;
	const type = tab === 'movies' || tab === 'shows' ? tab : tabOf(t);
	if (t.mood) return `/search?${new URLSearchParams({ mood: t.mood, genre: String(t.genre), type }).toString()}`;
	return `/explore?${new URLSearchParams(type === 'all' ? { genre: String(t.genre) } : { type, genre: String(t.genre) }).toString()}`;
}

/** Reads Search's mood view from the address; null when it is not one. */
export function readMood(p: Pick<URLSearchParams, 'get'>): MoodView | null {
	const mood = p.get('mood')?.trim();
	const genre = Number(p.get('genre'));
	const type = p.get('type');
	if (!mood || !Number.isInteger(genre) || genre <= 0 || (type !== 'all' && type !== 'movies' && type !== 'shows')) return null;
	return { mood, genre, type };
}

/** Tile hues: strong, distinct, readable under white type. */
export const TILE_HUES = ['#d0202a', '#b3123e', '#8e1f4f', '#6d2a8a', '#c2410c', '#9f1d35', '#3f6a8a', '#b91c1c', '#7a1d6b', '#2c7a7b', '#c0263f', '#a3402b'];

/** Gives a category its own colour, the same on every visit (by its name). */
export function tileHue(name: string): string {
	let h = 2166136261;
	for (const ch of name.toLowerCase()) h = Math.imul(h ^ ch.charCodeAt(0), 16777619);
	return TILE_HUES[(h >>> 0) % TILE_HUES.length];
}

/** The grid's column count at a viewport width (matches the CSS). */
export function browseCols(width: number): number {
	return width >= 1440 ? 6 : width >= 1280 ? 5 : width >= 900 ? 4 : width >= 600 ? 3 : 2;
}
