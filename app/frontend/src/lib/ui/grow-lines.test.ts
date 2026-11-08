import { describe, expect, it } from 'vitest';
import { contextLine, fitTags, growFacts, houseLine, shortRuntime, whyShown } from './grow-lines.ts';
import { shownScore } from '../core/rating.ts';

const words = (f: ReturnType<typeof growFacts>) => f.map((x) => x.text).join(' · ');

describe('growFacts', () => {
	it('reads a show as year, seasons, rating, stars', () => {
		expect(words(growFacts({ kind: 'show', year: 2024, seasons: 3, certification: 'TV-PG', score: 6.8 }))).toBe('2024 · 3 seasons · TV-PG · ★ 6.8');
		expect(words(growFacts({ kind: 'show', year: 2024, seasons: 1 }))).toBe('2024 · 1 season');
	});
	it('reads a movie as year, runtime, rating, stars', () => {
		expect(words(growFacts({ kind: 'movie', year: 2024, runtime: 106, certification: 'PG-13', score: 6.6 }))).toBe('2024 · 1h 46m · PG-13 · ★ 6.6');
	});
	it('hides the star under 25 votes (shared rating helper)', () => {
		expect(words(growFacts({ kind: 'movie', year: 2024, runtime: 90, score: shownScore(8.1, 12) }))).toBe('2024 · 1h 30m');
	});
	it('formats short and round runtimes', () => {
		expect(shortRuntime(45)).toBe('45m');
		expect(shortRuntime(120)).toBe('2h');
		expect(shortRuntime(0)).toBe('');
	});
});

describe('fitTags', () => {
	it('drops a mood on a Reality or Talk title (WWE Raw)', () => {
		expect(fitTags(['Inspiring'], ['Reality', 'Talk'])).toEqual(['Reality', 'Talk']);
		expect(fitTags(['Sports', 'Reality', 'Talk'], ['Reality', 'Talk'])).toEqual(['Sports', 'Reality', 'Talk']);
	});
	it('drops a mood the genres contradict', () => {
		expect(fitTags(['Scary', 'Witty'], ['Animation', 'Family'])).toEqual(['Witty', 'Animation']);
	});
	it('keeps fitting moods and tops up to two words', () => {
		expect(fitTags(['Suspenseful', 'Dark'], ['Thriller'])).toEqual(['Suspenseful', 'Dark']);
		expect(fitTags(['Scary'], ['Horror', 'Mystery'])).toEqual(['Scary', 'Horror']);
		expect(fitTags([], ['Drama', 'Comedy', 'Crime', 'War'])).toEqual(['Drama', 'Comedy']);
		expect(fitTags(undefined, undefined)).toEqual([]);
	});
});

describe('context line', () => {
	it('prefers the household status', () => {
		expect(contextLine({ state: 'available' }, { streaming: { name: 'Netflix', rank: 3 } })).toEqual({ text: 'In your Library · up to date', kind: 'house' });
		expect(houseLine({ state: 'downloading', label: 'Season 6 downloading' })).toBe('Season 6 downloading');
		expect(houseLine({ state: 'requested', askers: ['Mia'] })).toBe('Asked by Mia');
		expect(houseLine({ state: 'requested', mine: true })).toBe('You asked for it');
		expect(houseLine({ fresh: { kind: 'coming', text: 'Coming Nov 14' } })).toBe('Coming Nov 14');
	});
	it('else says why the card is shown', () => {
		expect(contextLine({}, { streaming: { name: 'Netflix', rank: 3 } })).toEqual({ text: '#3 on Netflix today', kind: 'why' });
		expect(whyShown({ rowType: 'because_have', rowTitle: 'Because you have Slow Horses' })).toBe('Because you have Slow Horses');
		expect(whyShown({ rowType: 'trending', rowTitle: 'Trending now', fresh: { kind: 'new_season', text: 'New season' } })).toBe('New season');
		expect(whyShown({ rowType: 'trending', rowTitle: 'Trending now' })).toBe('');
		expect(contextLine({}, {})).toBeNull();
	});
});
