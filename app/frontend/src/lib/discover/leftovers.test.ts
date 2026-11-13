import { describe, expect, it } from 'vitest';
import { reasonKey, shiftGuess, type Leftover, type MatchEpisode } from './leftovers.ts';

const left = (id: number, s: number, e: number, reason = 'episode_not_listed'): Leftover => ({
	file_id: id,
	relative_path: `Show/S${s}/x.S0${s}E${e}.mkv`,
	size: 1,
	reason,
	dropped_season: s,
	guess: { season: s, episodes: [e] },
	dismissed: false
});

function season(s: number, n: number, filled: number): MatchEpisode[] {
	return Array.from({ length: n }, (_, i) => ({ id: s * 100 + i + 1, season: s, episode: i + 1, hasFile: i < filled }));
}

describe('shiftGuess', () => {
	it('finds Season 2 -> Season 1, +12 after a merge', () => {
		const ls = Array.from({ length: 12 }, (_, i) => left(i + 1, 2, i + 1));
		const g = shiftGuess(ls, season(1, 24, 12));
		expect(g).not.toBeNull();
		expect([g!.from, g!.to, g!.shift]).toEqual([2, 1, 12]);
		expect(g!.pairs).toHaveLength(12);
		expect(g!.pairs[0]).toEqual({ file_id: 1, episode_id: 113, from: [2, 1], to: [1, 13] });
		expect(g!.pairs[11].to).toEqual([1, 24]);
	});

	it('gives no guess for a reorder inside one season', () => {
		const ls = [left(1, 1, 3), left(2, 1, 4)];
		expect(shiftGuess(ls, season(1, 6, 2))).toBeNull();
	});

	it('gives no guess when two seasons would fit', () => {
		const ls = [left(1, 3, 1), left(2, 3, 2)];
		expect(shiftGuess(ls, [...season(1, 4, 2), ...season(2, 4, 2)])).toBeNull();
	});

	it('gives no guess when the run of empty episodes is too short', () => {
		const ls = [left(1, 2, 1), left(2, 2, 2), left(3, 2, 3)];
		expect(shiftGuess(ls, season(1, 4, 2))).toBeNull();
	});

	it('offers the shift again after it was undone', () => {
		const ls = [left(1, 2, 1, 'unmatched'), left(2, 2, 2, 'unmatched')];
		const g = shiftGuess(ls, season(1, 7, 5));
		expect([g?.from, g?.to, g?.shift]).toEqual([2, 1, 5]);
	});

	it('skips an unmatched file whose episode is listed', () => {
		const ls = [left(1, 1, 3, 'unmatched'), left(2, 1, 4, 'unmatched')];
		expect(shiftGuess(ls, [...season(1, 4, 2), ...season(2, 6, 0)])).toBeNull();
	});

	it('skips dismissed files and other reasons', () => {
		const ls = [left(1, 2, 1), { ...left(2, 2, 2), dismissed: true }, left(3, 2, 3, 'no_episode_numbers')];
		expect(shiftGuess(ls, season(1, 4, 2))).toBeNull();
	});
});

describe('reasonKey', () => {
	it('names the old season for a dropped episode', () => {
		expect(reasonKey({ reason: 'episode_not_listed', dropped_season: 2 })).toBe('discover.leftovers.reason.season_not_listed');
		expect(reasonKey({ reason: 'episode_not_listed' })).toBe('discover.leftovers.reason.episode_not_listed');
	});
	it('falls back for an unknown key', () => {
		expect(reasonKey({ reason: 'something_new' })).toBe('discover.leftovers.reason.episode_not_listed');
		expect(reasonKey({ reason: 'movie_more_than_one' })).toBe('discover.leftovers.reason.movie_more_than_one');
	});
});
