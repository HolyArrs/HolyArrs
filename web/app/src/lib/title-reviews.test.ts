import { describe, expect, it } from 'vitest';
import { compactCount, ordersOf, pillState, reviewsPath, scoreText, spread, votesText, type ReviewSource } from './title-reviews.ts';

const tmdb: ReviewSource = { id: 'tmdb', name: 'TMDB', orders: ['newest'] };
const trakt: ReviewSource = { id: 'trakt', name: 'Trakt', orders: ['newest', 'helpful'] };

describe('review source pills', () => {
	it('TMDB is the default and Trakt shows only when the server lists it', () => {
		expect(pillState([tmdb], 'tmdb')).toEqual({ pills: [tmdb], active: 'tmdb' });
		expect(pillState([tmdb, trakt], 'tmdb').pills.map((p) => p.id)).toEqual(['tmdb', 'trakt']);
	});
	it('keeps the picked source while it exists, else falls back to TMDB', () => {
		expect(pillState([tmdb, trakt], 'trakt').active).toBe('trakt');
		expect(pillState([tmdb], 'trakt').active).toBe('tmdb');
		expect(pillState([trakt], 'tvdb').active).toBe('trakt');
		expect(pillState(undefined, 'trakt')).toEqual({ pills: [], active: 'tmdb' });
	});
	it('orders come from the source', () => {
		expect(ordersOf([tmdb, trakt], 'trakt')).toEqual(['newest', 'helpful']);
		expect(ordersOf([tmdb], 'tmdb')).toEqual(['newest']);
		expect(ordersOf([], 'x')).toEqual(['newest']);
	});
	it('builds the request path', () => {
		expect(reviewsPath('movie', '11', 'tmdb')).toBe('/metadata/movie/11/reviews?source=tmdb');
		expect(reviewsPath('show', '7', 'trakt', 3, 'helpful')).toBe('/metadata/show/7/reviews?source=trakt&page=3&order=helpful');
	});
});

describe('score words', () => {
	it('score and counts', () => {
		expect(scoreText(7.84)).toBe('7.8');
		expect(scoreText(8)).toBe('8.0');
		expect(scoreText(0)).toBe('');
		expect(compactCount(12400)).toBe('12.4K');
		expect(compactCount(950)).toBe('950');
		expect(compactCount(1_500_000)).toBe('1.5M');
		expect(compactCount(250_000)).toBe('250K');
		expect(votesText(1)).toBe('1 rating');
		expect(votesText(12400)).toBe('12.4K ratings');
	});
	it('spread runs 10 down to 1 as shares of the biggest bar', () => {
		const s = spread([0, 0, 0, 0, 0, 0, 0, 5, 10, 0]);
		expect(s[0]).toEqual({ point: 10, count: 0, share: 0 });
		expect(s[1]).toEqual({ point: 9, count: 10, share: 1 });
		expect(s[2].share).toBe(0.5);
		expect(spread([1, 2])).toEqual([]);
		expect(spread(undefined)).toEqual([]);
	});
});
