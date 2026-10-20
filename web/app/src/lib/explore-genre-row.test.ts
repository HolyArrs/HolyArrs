import { describe, expect, it } from 'vitest';
import { genreCardHref, genreRowPath, genreRowSlot } from './explore-genre-row.ts';

const rows = (...types: string[]) => types.map((type) => ({ type }));

describe('genre row', () => {
	it('opens the same genre page as the picker', () => {
		expect(genreCardHref({ kind: 'movie', id: 27 })).toBe('/explore?type=movies&genre=27');
		expect(genreCardHref({ kind: 'show', id: 10765 })).toBe('/explore?type=shows&genre=10765');
		expect(genreRowPath('all')).toBe('/explore/genre-row?type=all');
		// A merged genre (TV combines it) opens under All, movies and shows together.
		expect(genreCardHref({ kind: 'movie', id: 878, both: true })).toBe('/explore?genre=878');
	});

	it('sits after row 6, past the first personal rows', () => {
		expect(genreRowSlot(rows('top_services', 'top_picks', 'trending', 'because_have', 'best', 'because_asked', 'household', 'hidden'))).toBe(5);
	});

	it('never sits next to a genre row', () => {
		expect(genreRowSlot(rows('a', 'b', 'c', 'd', 'e', 'popular_genre', 'f', 'g'))).toBe(6);
		expect(genreRowSlot(rows('a', 'b', 'c', 'd', 'e', 'f', 'genre', 'g', 'h'))).toBe(7);
	});

	it('goes last on a short page and nowhere on an empty one', () => {
		expect(genreRowSlot(rows('a', 'b', 'c'))).toBe(2);
		expect(genreRowSlot([])).toBe(-1);
	});
});
