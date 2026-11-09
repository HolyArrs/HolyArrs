import { tasteMatch, whyShown } from './why-shown.ts';
import { describe, expect, it } from 'vitest';

describe('whyShown', () => {
	it('puts New season first, then Picked for you, at most two', () => {
		const w = whyShown({ kind: 'show', tmdb: '7', genres: ['Drama', 'Crime'], fresh: { kind: 'new_season', text: 'New Season' } }, { taste: ['Drama', 'Crime'] });
		expect(w.map((x) => x.text)).toEqual(['New season', 'Picked for you']);
	});
	it('says Picked for you on a taste match', () => {
		expect(whyShown({ kind: 'movie', tmdb: '1', genres: ['Drama', 'Crime'] }, { taste: ['Crime', 'Drama', 'Action'] }).map((x) => x.kind)).toEqual(['taste']);
		expect(whyShown({ kind: 'movie', tmdb: '1', genres: ['Drama'] }, { taste: ['Crime', 'Drama', 'Action'] })).toEqual([]);
	});
	it('ignores New season on movies', () => {
		expect(whyShown({ kind: 'movie', tmdb: '2', fresh: { kind: 'new_season', text: 'x' } })).toEqual([]);
	});
});

describe('tasteMatch', () => {
	it('needs one genre when the taste is a single genre', () => {
		expect(tasteMatch(['Horror'], ['Horror'])).toBe(true);
		expect(tasteMatch([], ['Horror'])).toBe(false);
	});
});
