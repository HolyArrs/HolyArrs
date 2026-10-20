import { describe, expect, it } from 'vitest';
import { browseCols, browseHref, browsePath, browseType, readMood, tileHue, TILE_HUES } from './search-browse.ts';

describe('search browse', () => {
	it('asks per tab, Movies and Shows only', () => {
		expect(browsePath('movies')).toBe('/explore/browse?type=movies');
		expect(browseType('movie')).toBe('movies');
		expect(browseType('show')).toBe('shows');
		expect(browseType('music')).toBe('all');
		expect(browseType('all')).toBe('all');
	});
	it('opens the genre page or the mood view', () => {
		expect(browseHref({ kind: 'movie', genre: 80 })).toBe('/explore?type=movies&genre=80');
		expect(browseHref({ kind: 'show', genre: 80, mood: 'True crime' })).toBe('/search?mood=True+crime&genre=80&type=shows');
	});
	it('reads the mood view back, and nothing else', () => {
		expect(readMood(new URLSearchParams('mood=Heist&genre=80&type=movies'))).toEqual({ mood: 'Heist', genre: 80, type: 'movies' });
		expect(readMood(new URLSearchParams('mood=Heist&genre=x&type=movies'))).toBeNull();
		expect(readMood(new URLSearchParams('genre=80&type=movies'))).toBeNull();
		expect(readMood(new URLSearchParams('mood=Heist&genre=80&type=music'))).toBeNull();
	});
	it('gives a category the same hue every time', () => {
		expect(tileHue('Crime')).toBe(tileHue('crime'));
		expect(TILE_HUES).toContain(tileHue('Heist'));
		expect(new Set(['Action', 'Comedy', 'Crime', 'Drama', 'Horror', 'Romance', 'Heist', 'Anime'].map(tileHue)).size).toBeGreaterThan(4);
	});
	it('matches the grid breakpoints', () => {
		expect([375, 700, 1024, 1300, 1440, 1920].map(browseCols)).toEqual([2, 3, 4, 5, 6, 6]);
	});
});
