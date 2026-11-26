import { describe, expect, it } from 'vitest';
import { episodeNamed, episodeWords } from './episodes.ts';

describe('episode words', () => {
	it('names an episode after its code', () => {
		expect(episodeWords(1, 2, 'Flood')).toBe('S01E02 · Flood');
		expect(episodeNamed('12 (S01E12)', ' The Bell ')).toBe('12 (S01E12) · The Bell');
	});

	it('leaves out unknown and placeholder names', () => {
		expect(episodeWords(1, 2)).toBe('S01E02');
		expect(episodeWords(1, 3, 'TBA')).toBe('S01E03');
		expect(episodeWords(undefined, 4, '')).toBe('E04');
	});
});
