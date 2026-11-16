import { describe, expect, it } from 'vitest';
import { seasonWatched, titleWatched, watchedIndex, watchedWords, type MyWatched } from './watched.ts';

const mine = (over: Partial<MyWatched> = {}): MyWatched => ({ enabled: true, linked: true, movies: [], shows: {}, ...over });
const show = (id: number, ...aired: number[]) => ({ id, kind: 'show', show: { seasons: aired.map((a, i) => ({ number: i, aired: a })) } });

describe('titleWatched', () => {
	it('marks a watched movie and leaves any other alone', () => {
		const ix = watchedIndex(mine({ movies: [4, 9] }));
		expect(titleWatched({ id: 9, kind: 'movie' }, ix)).toEqual({ state: 'watched', seen: 1, of: 1 });
		expect(titleWatched({ id: 5, kind: 'movie' }, ix)).toBeNull();
	});
	it('never reads a movie id as a show or an artist', () => {
		const ix = watchedIndex(mine({ movies: [7], shows: { '8': { '1': 2 } } }));
		expect(titleWatched(show(7, 0, 4), ix)).toBeNull();
		expect(titleWatched({ id: 8, kind: 'movie' }, ix)).toBeNull();
		expect(titleWatched({ id: 7, kind: 'artist' }, ix)).toBeNull();
	});
	it('marks a show watched once every aired episode of its seasons is seen', () => {
		const ix = watchedIndex(mine({ shows: { '3': { '1': 8, '2': 6 } } }));
		expect(titleWatched(show(3, 2, 8, 6), ix)).toEqual({ state: 'watched', seen: 14, of: 14 });
	});
	it('marks a show partly watched, counting each season up to what aired', () => {
		const ix = watchedIndex(mine({ shows: { '3': { '1': 9, '2': 1 } } }));
		expect(titleWatched(show(3, 0, 8, 6), ix)).toEqual({ state: 'partly', seen: 9, of: 14 });
	});
	it('leaves specials out of the count, yet a show with only a special seen is partly watched', () => {
		const ix = watchedIndex(mine({ shows: { '3': { '0': 1 } } }));
		expect(titleWatched(show(3, 2, 8), ix)).toEqual({ state: 'partly', seen: 0, of: 8 });
	});
	it('counts only the episodes in the Library, as Plex and Jellyfin do', () => {
		const ix = watchedIndex(mine({ shows: { '3': { '1': 2 } } }));
		const t = { id: 3, kind: 'show', show: { seasons: [{ number: 1, aired: 120, in_library: 3 }, { number: 2, aired: 66, in_library: 0 }] } };
		expect(titleWatched(t, ix)).toEqual({ state: 'partly', seen: 2, of: 3 });
		expect(seasonWatched(3, { number: 1, aired: 120, in_library: 3 }, ix)).toEqual({ state: 'partly', seen: 2, of: 3 });
	});
	it('says partly watched for a show whose seasons are not known', () => {
		const ix = watchedIndex(mine({ shows: { '3': { '1': 2 } } }));
		expect(titleWatched({ id: 3, kind: 'show' }, ix)).toEqual({ state: 'partly', seen: 2, of: 0 });
	});
	it('marks nothing while watch history is off, the person has no linked account, or nothing is loaded', () => {
		const t = { id: 9, kind: 'movie' };
		expect(titleWatched(t, watchedIndex(mine({ movies: [9], enabled: false })))).toBeNull();
		expect(titleWatched(t, watchedIndex(mine({ movies: [9], linked: false })))).toBeNull();
		expect(titleWatched(t, watchedIndex(null))).toBeNull();
	});
});

describe('seasonWatched', () => {
	const ix = watchedIndex(mine({ shows: { '3': { '1': 8, '2': 2, '0': 1 } } }));
	it('marks a season watched, partly watched or not at all', () => {
		expect(seasonWatched(3, { number: 1, aired: 8 }, ix)).toEqual({ state: 'watched', seen: 8, of: 8 });
		expect(seasonWatched(3, { number: 2, aired: 6 }, ix)).toEqual({ state: 'partly', seen: 2, of: 6 });
		expect(seasonWatched(3, { number: 3, aired: 6 }, ix)).toBeNull();
		expect(seasonWatched(4, { number: 1, aired: 8 }, ix)).toBeNull();
	});
	it('marks the specials on their own', () => {
		expect(seasonWatched(3, { number: 0, aired: 2 }, ix)).toEqual({ state: 'partly', seen: 1, of: 2 });
	});
	it('does not call a season watched before anything aired', () => {
		expect(seasonWatched(3, { number: 2, aired: 0 }, ix)).toEqual({ state: 'partly', seen: 2, of: 0 });
	});
});

describe('watchedWords', () => {
	const t = (key: string, vars?: Record<string, string | number>) => key + (vars ? ` ${vars.seen}/${vars.of}` : '');
	it('says watched, how much of it, or just partly when nothing is known to have aired', () => {
		expect(watchedWords({ state: 'watched', seen: 8, of: 8 }, t)).toBe('mediafiles.watched.all');
		expect(watchedWords({ state: 'partly', seen: 3, of: 8 }, t)).toBe('mediafiles.watched.part 3/8');
		expect(watchedWords({ state: 'partly', seen: 3, of: 0 }, t)).toBe('mediafiles.watched.partly');
	});
});
