import { afterEach, describe, expect, it, vi } from 'vitest';
import { heroSize, pick, preloadHero, proxied, sharpen, sized } from './tmdb-image.ts';
import { setBase } from './paths.ts';
import { setLook } from './look.svelte.ts';

describe('sized', () => {
	afterEach(() => setBase(''));
	it('asks the artwork cache for a w342 poster', () => {
		expect(sized('https://image.tmdb.org/t/p/original/a.jpg', 'poster')).toBe('/api/v1/artwork/tmdb/w342/a.jpg');
		expect(sized('https://image.tmdb.org/t/p/w500/a.jpg', 'poster')).toBe('/api/v1/artwork/tmdb/w342/a.jpg');
	});
	it('asks for a w780 backdrop', () => {
		expect(sized('https://image.tmdb.org/t/p/w1280/b.png', 'backdrop')).toBe('/api/v1/artwork/tmdb/w780/b.png');
	});
	it('keeps the URL base', () => {
		setBase('/holyarrs');
		expect(sized('https://image.tmdb.org/t/p/w500/a.jpg', 'poster')).toBe('/holyarrs/api/v1/artwork/tmdb/w342/a.jpg');
	});
	it('leaves other pictures alone', () => {
		expect(sized('/api/v1/artwork/x.jpg', 'poster')).toBe('/api/v1/artwork/x.jpg');
		expect(sized('https://image.tmdb.org/t/p/w500/../x.jpg', 'poster')).toBe('https://image.tmdb.org/t/p/w500/../x.jpg');
		expect(sized(null, 'poster')).toBe('');
	});
});

describe('pick', () => {
	it('takes the smallest slot size that covers the card at the pixel ratio', () => {
		expect(pick('backdrop', 260, 1)).toBe(780);
		expect(pick('backdrop', 260, 2)).toBe(780);
		expect(pick('poster', 168, 1)).toBe(185);
		expect(pick('poster', 168, 2)).toBe(342);
		expect(pick('poster', 80, 1)).toBe(185);
	});
	it('gives phone cards w500', () => {
		expect(pick('poster', 128, 3)).toBe(500);
		expect(pick('poster', 150, 2.75)).toBe(500);
	});
	it('never goes above the slot', () => {
		expect(pick('backdrop', 900, 2)).toBe(780);
		expect(pick('poster', 400, 3)).toBe(500);
		expect(pick('backdrop')).toBe(780);
		expect(pick('poster')).toBe(342);
	});
});

describe('hero backdrops', () => {
	it('size to the rendered width at the pixel ratio', () => {
		expect(heroSize(700, 1)).toBe('w1280');
		expect(heroSize(390, 3)).toBe('w1280');
		expect(heroSize(1536, 1)).toBe('w1280');
		expect(heroSize(1000, 2)).toBe('original');
		expect(heroSize(3440, 1)).toBe('original');
		expect(sharpen('/api/v1/artwork/tmdb/w780/b.jpg', 3440, 1)).toBe('/api/v1/artwork/tmdb/original/b.jpg');
		expect(sharpen('/x/y.jpg', 3440, 1)).toBe('/x/y.jpg');
	});
});

describe('proxied', () => {
	it('sends TMDB pictures through the artwork cache at a size it fetches', () => {
		expect(proxied('https://image.tmdb.org/t/p/w500/logo.png')).toMatch(/\/api\/v1\/artwork\/tmdb\/w500\/logo\.png$/);
		expect(proxied('https://image.tmdb.org/t/p/original/a_b-c.jpg')).toMatch(/\/api\/v1\/artwork\/tmdb\/original\/a_b-c\.jpg$/);
		expect(proxied('https://image.tmdb.org/t/p/h632/face.jpg')).toMatch(/\/api\/v1\/artwork\/tmdb\/h632\/face\.jpg$/);
		expect(proxied('https://image.tmdb.org/t/p/w45/tiny.jpg')).toMatch(/\/api\/v1\/artwork\/tmdb\/w500\/tiny\.jpg$/);
	});
	it('leaves other pictures and empty values alone', () => {
		expect(proxied('/api/v1/artwork/tmdb/w342/p.jpg')).toBe('/api/v1/artwork/tmdb/w342/p.jpg');
		expect(proxied('https://assets.fanart.tv/x.png')).toBe('https://assets.fanart.tv/x.png');
		expect(proxied(null)).toBe('');
	});
});

describe('preloadHero', () => {
	afterEach(() => {
		setLook('full');
		vi.unstubAllGlobals();
	});
	function stub() {
		const made: string[] = [];
		vi.stubGlobal('window', { innerWidth: 1000 });
		vi.stubGlobal('Image', class { decoding = ''; fetchPriority = ''; set src(u: string) { made.push(u); } });
		return made;
	}
	it('fetches the backdrop in the full look', () => {
		const made = stub();
		preloadHero('https://image.tmdb.org/t/p/original/full.jpg');
		expect(made).toEqual(['/api/v1/artwork/tmdb/w1280/full.jpg']);
	});
	it('fetches nothing in the minimal look', () => {
		const made = stub();
		setLook('minimal');
		preloadHero('https://image.tmdb.org/t/p/original/minimal.jpg');
		expect(made).toEqual([]);
	});
});
