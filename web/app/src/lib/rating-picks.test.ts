import { describe, expect, it } from 'vitest';
import { DEFAULT_RATINGS, shownRatings } from './rating-picks.svelte.ts';

const s = (source: string, value: number, max = 100) => ({ source, value, max, votes: 100 });

describe('shown ratings', () => {
	it('shows the picked ones a title has, in the fixed order', () => {
		const all = [s('imdb', 8.4, 10), s('rt_critics', 94), s('rt_audience', 94), s('metacritic', 89)];
		expect(shownRatings(all, 8.4, DEFAULT_RATINGS).scores.map((x) => x.source)).toEqual(['imdb', 'rt_critics', 'rt_audience']);
		expect(shownRatings(all, 8.4, DEFAULT_RATINGS).tmdb).toBeUndefined();
	});
	it('adds TMDB only when picked', () => {
		expect(shownRatings([], 7.1, ['tmdb']).tmdb).toBe(7.1);
	});
	it('falls back to one score when none of the picked ones is there', () => {
		expect(shownRatings([s('trakt', 80)], 6.5, ['imdb']).scores.map((x) => x.source)).toEqual(['trakt']);
		expect(shownRatings([], 6.5, ['imdb'])).toEqual({ scores: [], tmdb: 6.5 });
		expect(shownRatings([], undefined, ['imdb'])).toEqual({ scores: [], tmdb: undefined });
	});
});
