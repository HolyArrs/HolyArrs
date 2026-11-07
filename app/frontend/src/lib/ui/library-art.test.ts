import { describe, expect, it } from 'vitest';
import { bannerHeight, bannerRatio, posterMinFor, posterRatio, bannerSrc, bannerWidth, loadDensity, loadPosterSize, saveDensity, savePosterSize, posterSrc, tmdbFor } from './library-art.ts';

function mem() {
	const m = new Map<string, string>();
	return { getItem: (k: string) => m.get(k) ?? null, setItem: (k: string, v: string) => void m.set(k, v) };
}

describe('banner width', () => {
	it('is 30% of the table, clamped 420 to 640, for Large art', () => {
		expect(bannerWidth(1000, 'large')).toBe(420);
		expect(bannerWidth(1800, 'large')).toBe(540);
		expect(bannerWidth(3400, 'large')).toBe(640);
	});
	it('shrinks for the other densities', () => {
		expect(bannerWidth(1800, 'comfortable')).toBeLessThan(bannerWidth(1800, 'large'));
		expect(bannerWidth(1800, 'compact')).toBeLessThan(bannerWidth(1800, 'comfortable'));
	});
	it('takes its height from the 1000x185 shape', () => {
		expect(bannerHeight(540)).toBe(100);
		expect(bannerHeight(540, 'movie')).toBe(100);
		expect(bannerHeight(540, 'show')).toBe(100);
	});
	it('is taller for Music (1000x400)', () => {
		expect(bannerHeight(500, 'artist')).toBe(200);
		expect(bannerRatio('artist')).toBeCloseTo(1000 / 400);
		expect(bannerRatio('movie')).toBeCloseTo(1000 / 185);
	});
});

describe('poster shape', () => {
	it('is square for Music and 2:3 for movies and shows', () => {
		expect(posterRatio('artist')).toBe('1 / 1');
		expect(posterRatio('movie')).toBe('2 / 3');
		expect(posterRatio('show')).toBe('2 / 3');
	});
	it('packs Music a little tighter and keeps the rest', () => {
		expect(posterMinFor('m', 'movie')).toBe(172);
		expect(posterMinFor('m', 'artist')).toBeLessThan(172);
		expect(posterMinFor('s', 'artist')).toBeLessThan(posterMinFor('l', 'artist'));
	});
});

describe('banner picture size', () => {
	it('covers display width times pixel ratio', () => {
		expect(tmdbFor(420, 1)).toBe('w780');
		expect(tmdbFor(700, 2)).toBe('original');
		expect(tmdbFor(640, 1.5)).toBe('w1280');
		const u = 'https://image.tmdb.org/t/p/w780/abc.jpg';
		expect(bannerSrc(u, 700, 2)).toBe('/api/v1/artwork/tmdb/original/abc.jpg');
		expect(bannerSrc('/api/v1/artwork/tmdb/w300/abc.jpg', 600, 2)).toBe('/api/v1/artwork/tmdb/w1280/abc.jpg');
	});
	it('leaves a stored banner as it is', () => {
		expect(bannerSrc('/api/v1/artwork/img/title-banner/tv-1/1000.webp', 600)).toBe('/api/v1/artwork/img/title-banner/tv-1/1000.webp');
	});
});

describe('remembered per member', () => {
	it('keeps density and poster size apart for each member', () => {
		const s = mem();
		expect(loadDensity(1, s)).toBe('large');
		expect(loadPosterSize(1, s)).toBe('m');
		saveDensity(1, 'compact', s);
		savePosterSize(1, 'l', s);
		expect(loadDensity(1, s)).toBe('compact');
		expect(loadPosterSize(1, s)).toBe('l');
		expect(loadDensity(2, s)).toBe('large');
		expect(loadPosterSize(2, s)).toBe('m');
	});
	it('ignores junk', () => {
		const s = mem();
		s.setItem('holyarrs.library-density.1', 'huge');
		expect(loadDensity(1, s)).toBe('large');
	});
});

describe('posterSrc', () => {
	const u = 'https://image.tmdb.org/t/p/w342/abc.jpg';
	it('keeps small posters light at 1x', () => expect(posterSrc(u, 132, 1)).toContain('/w342/'));
	it('sharpens large posters on 2x screens', () => expect(posterSrc(u, 232, 2)).toContain('/w780/'));
	it('uses the original on 3x screens', () => expect(posterSrc(u, 232, 3)).toContain('/original/'));
});

import { tmdbSized } from './library-art.ts';
import { describe as d2, it as i2, expect as e2 } from 'vitest';
d2('tmdbSized', () => {
	const u = 'https://image.tmdb.org/t/p/original/abc.jpg';
	i2('picks the smallest size of the kind that covers the slot', () => {
		e2(tmdbSized(u, 'profile', 92, 2)).toContain('/w185/');
		e2(tmdbSized(u, 'profile', 400, 2)).toContain('/h632/');
		e2(tmdbSized(u, 'poster', 44, 2)).toContain('/w92/');
		e2(tmdbSized(u, 'backdrop', 375, 2)).toContain('/w780/');
		e2(tmdbSized(u, 'logo', 420, 1)).toContain('/w500/');
		e2(tmdbSized('/x.png', 'logo', 420, 1)).toBe('/x.png');
	});
});
