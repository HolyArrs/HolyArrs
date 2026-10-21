import { describe, expect, it } from 'vitest';
import { tasteMatch, top10Map, whyShown } from './why-shown.ts';

describe('whyShown', () => {
	const top10 = top10Map([{ kind: 'show', items: [{ tmdb_id: '7', rank: 4 }] }]);
	it('puts New season first, then the Top 10 rank, at most two', () => {
		const w = whyShown({ kind: 'show', tmdb: '7', genres: ['Drama', 'Crime'], fresh: { kind: 'new_season', text: 'New Season' } }, { top10, top10Source: 'Netflix', taste: ['Drama', 'Crime'] });
		expect(w.map((x) => x.text)).toEqual(['New season', '#4 in TV shows on Netflix']);
	});
	it('says Picked for you on a taste match', () => {
		expect(whyShown({ kind: 'movie', tmdb: '1', genres: ['Drama', 'Crime'] }, { taste: ['Crime', 'Drama', 'Action'] }).map((x) => x.kind)).toEqual(['taste']);
		expect(whyShown({ kind: 'movie', tmdb: '1', genres: ['Drama'] }, { taste: ['Crime', 'Drama', 'Action'] })).toEqual([]);
	});
	it('falls back to the fresh Top 10 rank and ignores New season on movies', () => {
		expect(whyShown({ kind: 'movie', tmdb: '2', fresh: { kind: 'top10', text: 'Trending', rank: 3 } }).map((x) => x.text)).toEqual(['#3 in Movies']);
		expect(whyShown({ kind: 'movie', tmdb: '2', fresh: { kind: 'new_season', text: 'x' } })).toEqual([]);
	});
});

describe('tasteMatch', () => {
	it('needs one genre when the taste is a single genre', () => {
		expect(tasteMatch(['Horror'], ['Horror'])).toBe(true);
		expect(tasteMatch([], ['Horror'])).toBe(false);
	});
});

describe('top10Map', () => {
	it('keeps the best rank per title', () => {
		const m = top10Map([{ items: [{ kind: 'movie', tmdb_id: 5, rank: 6 }, { kind: 'movie', tmdb_id: 5, rank: 2 }] }, null]);
		expect(m.get('movie:5')).toBe(2);
	});
});
