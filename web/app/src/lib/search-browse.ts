import type { ExploreType } from './explore.ts';

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
}

export interface BrowseAnswer {
	type: ExploreType;
	tiles: BrowseTile[];
}

export function browsePath(t: ExploreType): string {
	return `/explore/browse?${new URLSearchParams({ type: t }).toString()}`;
}

/** The Browse grid's tab for a search scope: Movies and Shows only, both under any other scope. */
export function browseType(scope: string): ExploreType {
	return scope === 'movie' ? 'movies' : scope === 'show' ? 'shows' : 'all';
}

const tabOf = (t: Pick<BrowseTile, 'kind' | 'both'>): 'all' | 'movies' | 'shows' => (t.both ? 'all' : t.kind === 'show' ? 'shows' : 'movies');

/** browseHref: a genre tile opens the Explore genre page, a mood tile Search's mood view. */
export function browseHref(t: Pick<BrowseTile, 'kind' | 'genre' | 'mood' | 'both'>): string {
	const type = tabOf(t);
	if (t.mood) return `/search?${new URLSearchParams({ mood: t.mood, genre: String(t.genre), type }).toString()}`;
	return `/explore?${new URLSearchParams(type === 'all' ? { genre: String(t.genre) } : { type, genre: String(t.genre) }).toString()}`;
}

export interface MoodView {
	mood: string;
	genre: number;
	type: 'all' | 'movies' | 'shows';
}

/** readMood reads Search's mood view from the address; null when it is not one. */
export function readMood(p: Pick<URLSearchParams, 'get'>): MoodView | null {
	const mood = p.get('mood')?.trim();
	const genre = Number(p.get('genre'));
	const type = p.get('type');
	if (!mood || !Number.isInteger(genre) || genre <= 0 || (type !== 'all' && type !== 'movies' && type !== 'shows')) return null;
	return { mood, genre, type };
}

/** The tile hues: strong, distinct, readable under white type. */
export const TILE_HUES = ['#d1344b', '#e0702a', '#c99a1c', '#5f9e2f', '#1f9e7a', '#1b8fb8', '#2f6fd8', '#5b4fd6', '#8e44c9', '#c23f9b', '#b5523a', '#3d7f8c', '#7a8b2a', '#a8376a'];

/** tileHue gives a category its own colour, the same on every visit (by its name). */
export function tileHue(name: string): string {
	let h = 2166136261;
	for (const ch of name.toLowerCase()) h = Math.imul(h ^ ch.charCodeAt(0), 16777619);
	return TILE_HUES[(h >>> 0) % TILE_HUES.length];
}

/** browseCols is the grid's column count at a viewport width (matches the CSS). */
export function browseCols(width: number): number {
	return width >= 1440 ? 6 : width >= 1280 ? 5 : width >= 900 ? 4 : width >= 600 ? 3 : 2;
}
