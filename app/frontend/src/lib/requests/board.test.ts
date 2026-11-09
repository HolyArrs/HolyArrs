import { askHref, detailKey, kindWord, stepIndex } from './board.ts';
import { describe, expect, it } from 'vitest';

const who = (id: number, name: string) => ({ id, name });

describe('requests page', () => {
	it('links each ask to its Library page, title page or artist page', () => {
		expect(askHref({ kind: 'movie', tmdb_id: '496243', title_id: 12 })).toBe('/library/12');
		expect(askHref({ kind: 'movie', tmdb_id: '496243' })).toBe('/title/movie/496243');
		expect(askHref({ kind: 'show', tmdb_id: '119051' })).toBe('/title/show/119051');
		expect(askHref({ kind: 'artist', tmdb_id: '', ref: 'mbid:63aa26c3' })).toBe('/music/artist/63aa26c3');
		expect(askHref({ kind: 'artist', tmdb_id: '' })).toBe('');
		expect(askHref({ kind: 'movie', tmdb_id: '1', title_id: -1 })).toBe('/title/movie/1');
	});

	it('wraps the focus position', () => {
		expect(stepIndex(0, -1, 8)).toBe(7);
		expect(stepIndex(7, 1, 8)).toBe(0);
		expect(stepIndex(3, 1, 0)).toBe(0);
	});

	it('says small words', () => {
		expect(kindWord('show')).toBe('TV show');
		expect(kindWord('artist')).toBe('Music');
		expect(kindWord('movie')).toBe('Movie');
		expect(detailKey({ kind: 'artist', tmdb_id: '' })).toBe('');
		expect(detailKey({ kind: 'show', tmdb_id: '5' })).toBe('show:5');
	});
});
