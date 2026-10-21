import { describe, expect, it } from 'vitest';
import { bannerHeight, bannerSrc, bannerWidth, loadDensity, loadPosterSize, saveDensity, savePosterSize, tmdbFor } from './library-art.ts';

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
	});
});

describe('banner picture size', () => {
	it('covers display width times pixel ratio', () => {
		expect(tmdbFor(420, 1)).toBe('w780');
		expect(tmdbFor(700, 2)).toBe('original');
		expect(tmdbFor(640, 1.5)).toBe('w1280');
		const u = 'https://image.tmdb.org/t/p/w780/abc.jpg';
		expect(bannerSrc(u, 700, 2)).toBe('https://image.tmdb.org/t/p/original/abc.jpg');
		expect(bannerSrc('/api/v1/artwork/tmdb/w300/abc.jpg', 600, 2)).toBe('/api/v1/artwork/tmdb/w1280/abc.jpg');
	});
	it('uses the full fanart file, never the preview', () => {
		expect(bannerSrc('https://assets.fanart.tv/preview/tv/1/tvbanner/x.jpg', 600)).toBe('https://assets.fanart.tv/fanart/tv/1/tvbanner/x.jpg');
		expect(bannerSrc('https://assets.fanart.tv/fanart/tv/1/tvbanner/x.jpg', 600)).toBe('https://assets.fanart.tv/fanart/tv/1/tvbanner/x.jpg');
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
