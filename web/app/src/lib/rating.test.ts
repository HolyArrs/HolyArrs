import { describe, expect, it } from 'vitest';
import { MIN_VOTES, shownScore } from './rating.ts';

describe('shownScore', () => {
	it('shows a score with enough votes', () => {
		expect(shownScore(7.4, MIN_VOTES)).toBe(7.4);
		expect(shownScore(8, 12000)).toBe(8);
	});
	it('hides a score with fewer than 25 votes', () => {
		expect(shownScore(9.5, 24)).toBeUndefined();
		expect(shownScore(9.5, 0)).toBeUndefined();
	});
	it('hides when the votes or score are missing', () => {
		expect(shownScore(7, undefined)).toBeUndefined();
		expect(shownScore(undefined, 500)).toBeUndefined();
		expect(shownScore(0, 500)).toBeUndefined();
		expect(shownScore('7', 500)).toBeUndefined();
	});
});
