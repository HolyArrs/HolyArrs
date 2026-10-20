import type { ExploreType } from './explore.ts';

/** One card of the "Browse by genre" row (GET /explore/genre-row). */
export interface GenreRowCard {
	kind: 'movie' | 'show';
	id: number;
	name: string;
	backdrop: string;
	/** The genre page holds movies and shows: it opens under All (id is the movie genre's). */
	both?: boolean;
	/** The title the backdrop comes from (kind:tmdb id). */
	from: string;
}

export interface GenreRowAnswer {
	type: ExploreType;
	genres: GenreRowCard[];
}

/** The row sits after this many rows (counting Trending on streaming): past the first personal rows. */
export const GENRE_ROW_AT = 6;

/** Rows about one genre: the browse row never sits right under one. */
const GENRE_TYPES = new Set(['genre', 'popular_genre', 'mood']);

export function genreRowPath(t: ExploreType): string {
	return `/explore/genre-row?${new URLSearchParams({ type: t }).toString()}`;
}

/** genreTab is the tab whose genre page a card opens. */
export const genreTab = (c: Pick<GenreRowCard, 'kind' | 'both'>): 'all' | 'movies' | 'shows' => (c.both ? 'all' : c.kind === 'show' ? 'shows' : 'movies');

/** genreCardHref is the Explore genre page a card opens: the same URL the Genres picker sets. */
export function genreCardHref(c: Pick<GenreRowCard, 'kind' | 'id' | 'both'>): string {
	const type = genreTab(c);
	return `/explore?${new URLSearchParams(type === 'all' ? { genre: String(c.id) } : { type, genre: String(c.id) }).toString()}`;
}

/**
 * genreRowSlot is the index of the row the browse row follows: row GENRE_ROW_AT
 * (moved one down when that row is itself a genre row), or the last row on a short page;
 * -1 when the page has no rows.
 */
export function genreRowSlot(rows: { type?: string }[]): number {
	if (rows.length === 0) return -1;
	let at = Math.min(GENRE_ROW_AT, rows.length) - 1;
	while (at < rows.length - 1 && (GENRE_TYPES.has(rows[at].type ?? '') || GENRE_TYPES.has(rows[at + 1].type ?? ''))) at++;
	return at;
}
