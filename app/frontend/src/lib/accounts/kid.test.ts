import { isKid, kidLabel, ratingChoices } from './kid.ts';
import { describe, expect, it } from 'vitest';

describe('kid ratings', () => {
	it('offers G, PG, PG-13 and R with the show rating beside each', () => {
		expect(ratingChoices.map((c) => [c.value, c.show])).toEqual([
			['G', 'TV-G'],
			['PG', 'TV-PG'],
			['PG-13', 'TV-14'],
			['R', 'TV-MA']
		]);
	});
	it('labels a limit in plain words', () => {
		expect(kidLabel('PG')).toBe('Kid · up to PG');
		expect(kidLabel('')).toBe('');
		expect(kidLabel(undefined)).toBe('');
	});
	it('a Kid is anyone with a limit', () => {
		expect(isKid({ max_rating: 'PG' })).toBe(true);
		expect(isKid({ max_rating: '' })).toBe(false);
		expect(isKid({})).toBe(false);
		expect(isKid(null)).toBe(false);
	});
	it('describes the limit for the hint', () => {
		expect(ratingChoices.find((c) => c.value === 'PG')?.hint).toBe('Movies up to PG and shows up to TV-PG. Titles without a rating and music stay hidden.');
	});
});
