import { describe, expect, it } from 'vitest';
import { cleanupQuery, watchedLine, type WatchedCleanup } from './watched-cleanup.ts';

const people = [
	{ key: '1:22', name: 'ana', server: 'Plex', chosen: true },
	{ key: '2:u 1', name: 'bo', server: 'Jelly', chosen: true }
];
const base: WatchedCleanup = { enabled: true, days: 30, people, found: [] };
const one = { id: 1, kind: 'movie' as const, name: 'Dune', bytes: 2048, last_watched_at: '2026-01-01T00:00:00Z' };

describe('cleanupQuery', () => {
	it.each([
		{ name: 'everyone chosen sends no people', days: 30, picked: ['1:22', '2:u 1'], want: '?days=30' },
		{ name: 'some chosen', days: 7, picked: ['1:22'], want: '?days=7&people=1%3A22' },
		{ name: 'odd keys are escaped', days: 0, picked: ['2:u 1'], want: '?days=0&people=2%3Au+1' },
		{ name: 'nobody chosen', days: 30, picked: [], want: '' }
	])('$name', ({ days, picked, want }) => {
		expect(cleanupQuery(days, picked, people)).toBe(want);
	});
});

describe('watchedLine', () => {
	it.each([
		{ name: 'not loaded', c: null, has: '' },
		{ name: 'nothing found', c: base, has: '30' },
		{ name: 'found', c: { ...base, found: [one] }, has: '2.0 KB' }
	])('$name', ({ c, has }) => {
		const line = watchedLine(c);
		if (has) expect(line).toContain(has);
		else expect(line).toBe('');
	});
});
