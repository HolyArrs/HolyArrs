import { describe, expect, it } from 'vitest';
import { browseCols, browseHref, browsePath, browseType, musicTiles, readMood, strictTiles, tileHue, TILE_HUES } from './search-browse.ts';

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
	it('opens a merged genre under All', () => {
		expect(browseHref({ kind: 'movie', genre: 878, both: true })).toBe('/explore?genre=878');
		expect(browseHref({ kind: 'movie', genre: 14, mood: 'Dragons', both: true })).toBe('/search?mood=Dragons&genre=14&type=all');
		expect(readMood(new URLSearchParams('mood=Dragons&genre=14&type=all'))).toEqual({ mood: 'Dragons', genre: 14, type: 'all' });
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

describe('strict Browse per type', () => {
	const t = (kind: 'movie' | 'show', both = false) => ({ kind, genre: 1, genre_name: 'Drama', name: 'Drama', backdrop: 'b', from: 'f', both });
	it('keeps only the tab own kind; All keeps every tile', () => {
		const all = [t('movie'), t('show'), t('movie', true)];
		expect(strictTiles(all, 'movies').every((x) => x.kind === 'movie')).toBe(true);
		expect(strictTiles(all, 'shows')).toHaveLength(1);
		expect(strictTiles(all, 'all')).toHaveLength(3);
	});
	it('links under a tab always open that tab', () => {
		expect(browseHref(t('show', true), 'shows')).toBe('/explore?type=shows&genre=1');
		expect(browseHref(t('movie'), 'movies')).toBe('/explore?type=movies&genre=1');
	});
	it('music tiles use a picture of the genre, each once', () => {
		const card = (key: string, image: string, genres: string[]) => ({ key, kind: 'artist' as const, mbid: key, name: key, image, genres });
		const out = musicTiles({ genres: ['rock', 'jazz', 'folk'], hero: null, rows: [{ id: 'r', type: 'x', title: 'x', reason: '', shape: 'round', items: [card('a', '/a.jpg', ['rock']), card('b', '/b.jpg', ['rock', 'jazz'])] }] }, (g) => `/m/${g}`, (c) => c.image ?? null);
		expect(out.map((x) => [x.name, x.image, x.href])).toEqual([['Rock', '/a.jpg', '/m/rock'], ['Jazz', '/b.jpg', '/m/jazz']]);
	});
});
