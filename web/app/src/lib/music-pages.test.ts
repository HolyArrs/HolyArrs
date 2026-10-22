import { describe, expect, it } from 'vitest';
import { albumHref, artistHref, coverFor, haveLine, kindWord, length, owned, shelves, type PageAlbum } from './music-pages.ts';

const al = (over: Partial<PageAlbum>): PageAlbum => ({ mbid: 'm', name: 'N', type: 'album', secondary_types: [], shelf: 'albums', image: '', ...over });

describe('music pages', () => {
	it('sizes Cover Art Archive covers 250 or 500 by display size', () => {
		const u = 'https://coverartarchive.org/release-group/abc/front';
		expect(coverFor(u, 200, 1)).toEqual({ src: `${u}-250`, srcset: `${u}-250 250w, ${u}-500 500w` });
		expect(coverFor(`${u}-250`, 200, 2).src).toBe(`${u}-500`);
		const p = '/api/v1/music-trending/art/album/abc';
		expect(coverFor(p, 200, 1)).toEqual({ src: `${p}?size=250`, srcset: `${p}?size=250 250w, ${p}?size=500 500w` });
		expect(coverFor(p, 200, 2).src).toBe(`${p}?size=500`);
		expect(coverFor(p, 320, 2).src).toBe(`${p}?size=1200`);
		expect(coverFor('/api/v1/music-trending/art/artist/abc', 200)).toEqual({ src: '/api/v1/music-trending/art/artist/abc' });
	});
	it('links artists and albums by MusicBrainz id', () => {
		expect(artistHref('abc')).toMatch(/\/music\/artist\/abc$/);
		expect(albumHref('x', 'y')).toMatch(/\/music\/album\/x\?artist=y$/);
	});
	it('maps Library states to the card pill', () => {
		expect(owned(al({ state: 'complete' }))?.state).toBe('available');
		expect(owned(al({ state: 'downloading' }))?.state).toBe('downloading');
		expect(owned(al({ state: 'missing' }))?.label).toMatch(/^Missing/);
		expect(owned(al({ state: 'not_included' }))).toBeNull();
		expect(owned(al({}), true)?.label).toBe('Requested');
	});
	it('splits shelves and leaves coming albums out', () => {
		const s = shelves([al({ mbid: 'a' }), al({ mbid: 'b', shelf: 'live' }), al({ mbid: 'c', upcoming: true })]);
		expect(s.map((x) => [x.shelf, x.albums.length])).toEqual([['albums', 1], ['live', 1]]);
	});
	it('words lengths, kinds and the have line', () => {
		expect(length(187000)).toBe('3:07');
		expect(length(4320000, true)).toBe('1 hr 12 min');
		expect(kindWord({ type: 'ep', secondary_types: [] })).toBe('EP');
		expect(kindWord({ type: 'album', secondary_types: ['live'] })).toBe('Live');
		expect(haveLine(3, 9)).toBe('On HolyArrs: 3 of 9 releases');
	});
});
