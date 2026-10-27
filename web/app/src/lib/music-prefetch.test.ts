import { describe, expect, it, vi } from 'vitest';

vi.mock('./api.ts', () => ({ api: { get: vi.fn() } }));
const { createMusicPrefetch, IN_FLIGHT, musicPrefetchURLs, pagePrefetchURLs, PER_MINUTE, VIEW_CARDS } = await import('./music-prefetch.ts');
const { loadDetails } = await import('./title-details.ts');

const A = 'f9133869-f87c-459c-9aa7-2f176cda7e06';
const B = '9fff2f8a-21e6-47de-a2b8-7f449929d43f';
const artist = (i: number) => `/music/artist/${String(i).padStart(8, '0')}-0000-0000-0000-000000000000`;
const flush = async () => {
	for (let i = 0; i < 6; i++) await Promise.resolve();
};

describe('musicPrefetchURLs', () => {
	it('names the artist page core and full answers', () => {
		expect(musicPrefetchURLs(`/music/artist/${A}`)).toEqual([`/music/artists/${A}?part=core`, `/music/artists/${A}?similar=0`]);
		expect(musicPrefetchURLs(`/app/music/artist/${A}?tab=library`)).toHaveLength(2);
	});
	it('names the album core answer, lowercased like the page, only with an artist', () => {
		expect(musicPrefetchURLs(`/music/album/${B.toUpperCase()}?artist=${A.toUpperCase()}`)).toEqual([`/music/albums/${B}?part=core&artist=${A}`]);
		expect(musicPrefetchURLs(`/music/album/${B}`)).toEqual([]);
	});
	it('ignores other links', () => {
		expect(musicPrefetchURLs('/title/movie/1')).toEqual([]);
		expect(musicPrefetchURLs('/music/artist/not-an-id')).toEqual([]);
		expect(musicPrefetchURLs(null)).toEqual([]);
	});
});

describe('pagePrefetchURLs', () => {
	it('names the metadata and lookup, as loadDetails asks them', async () => {
		const asked: string[] = [];
		const get = <T,>(p: string) => (asked.push(p), Promise.resolve({ movies: [], shows: [] } as T));
		await loadDetails(get, 'movie', '693134').catch(() => {});
		await loadDetails(get, 'show', '1396').catch(() => {});
		expect(pagePrefetchURLs('/title/movie/693134')).toEqual(asked.slice(0, 2));
		expect(pagePrefetchURLs('/title/show/1396?view=2')).toEqual(asked.slice(2, 4));
	});
	it('names a Library title, and keeps music links as before', () => {
		expect(pagePrefetchURLs('/library/294')).toEqual(['/titles/294']);
		expect(pagePrefetchURLs('/library/294?season=2')).toEqual(['/titles/294']);
		expect(pagePrefetchURLs(`/music/artist/${A}`)).toEqual(musicPrefetchURLs(`/music/artist/${A}`));
	});
	it('ignores other links', () => {
		for (const h of ['/library', '/library/held', '/library/3/albums/9', '/title/tv/1', '/people/287', null]) expect(pagePrefetchURLs(h)).toEqual([]);
	});
});

describe('createMusicPrefetch', () => {
	it('fetches a hovered link once while fresh', () => {
		const get = vi.fn(() => new Promise<unknown>(() => {}));
		let t = 0;
		const p = createMusicPrefetch(get, () => t);
		expect(p.hover(`/music/artist/${A}`)).toBe(2);
		t = 30_000;
		expect(p.hover(`/music/artist/${A}`)).toBe(0);
		expect(get).toHaveBeenCalledTimes(2);
		t = 61_000;
		expect(p.hover(`/music/artist/${A}`)).toBe(2);
	});

	it('runs a few at a time and puts hovers first', async () => {
		const done: (() => void)[] = [];
		const get = vi.fn(() => new Promise<unknown>((r) => done.push(() => r({}))));
		const p = createMusicPrefetch(get, () => 0);
		p.view([artist(1), artist(2), artist(3)]);
		expect(get).toHaveBeenCalledTimes(IN_FLIGHT);
		p.hover(`/music/album/${B}?artist=${A}`);
		done.shift()?.();
		await flush();
		expect(get).toHaveBeenLastCalledWith(`/music/albums/${B}?part=core&artist=${A}`);
	});

	it('takes only the first cards on screen per page', () => {
		const get = vi.fn(() => new Promise<unknown>(() => {}));
		const p = createMusicPrefetch(get, () => 0);
		const links = ['/title/movie/1', ...Array.from({ length: 10 }, (_, i) => artist(i))];
		p.view(links);
		p.view(links);
		expect(p.running() + p.pending()).toBe(VIEW_CARDS * 2);
		p.newPage();
		p.view([artist(20)]);
		expect(p.running() + p.pending()).toBe(VIEW_CARDS * 2 + 2);
	});

	it('stops at the per-minute cap', () => {
		const get = vi.fn(() => Promise.resolve({}));
		let t = 0;
		const p = createMusicPrefetch(get, () => t);
		let n = 0;
		for (let i = 0; i < PER_MINUTE; i++) n += p.hover(artist(i));
		expect(n).toBe(PER_MINUTE);
		t = 60_000;
		expect(p.hover(artist(500))).toBe(2);
	});

	it('asks again after a failure, and after forget', async () => {
		const get = vi.fn(() => Promise.reject(new Error('down')));
		const p = createMusicPrefetch(get, () => 0);
		p.hover(`/music/album/${B}?artist=${A}`);
		await flush();
		expect(p.hover(`/music/album/${B}?artist=${A}`)).toBe(1);
		const ok = vi.fn(() => Promise.resolve({}));
		const q = createMusicPrefetch(ok, () => 0);
		q.hover(`/music/artist/${A}`);
		q.forget();
		expect(q.hover(`/music/artist/${A}`)).toBe(2);
	});
});
