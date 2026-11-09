import { albumHref, artistHref, artistKind, countryName, coverFor, kindWord, length, owned, shelves, withCover, type PageAlbum } from './music-pages.ts';
import { describe, expect, it } from 'vitest';

const al = (over: Partial<PageAlbum>): PageAlbum => ({ mbid: 'm', name: 'N', type: 'album', secondary_types: [], shelf: 'albums', image: '', ...over });

describe('music pages', () => {
	it('keeps only albums and artists with a real picture in a row', () => {
		const row = [al({ mbid: 'a', image: '/api/v1/x' }), al({ mbid: 'b', image: '' }), al({ mbid: 'c', image: '  ' })];
		expect(withCover(row).map((a) => a.mbid)).toEqual(['a']);
		expect(withCover(null)).toEqual([]);
	});
	it('asks one size per slot: 500 for cards, 250 for thumbnails, 1200 only for heroes', () => {
		const p = '/api/v1/music-trending/art/album/abc';
		expect(coverFor(p, 200, 1)).toEqual({ src: `${p}?size=500` });
		expect(coverFor(p, 200, 3).src).toBe(`${p}?size=500`);
		expect(coverFor(p, 430, 2).src).toBe(`${p}?size=500`);
		expect(coverFor(p, 44, 2).src).toBe(`${p}?size=250`);
		expect(coverFor(p, 120, 3).src).toBe(`${p}?size=500`);
		expect(coverFor(p, 260, 1, true).src).toBe(`${p}?size=500`);
		expect(coverFor(p, 260, 2, true).src).toBe(`${p}?size=1200`);
		expect(coverFor(`${p}?pv=c1`, 200, 2).src).toBe(`${p}?pv=c1&size=500`);
		const u = 'https://coverartarchive.org/release-group/abc/front';
		expect(coverFor(`${u}-250`, 200, 2).src).toBe(`${u}-500`);
		const dz = 'https://cdn-images.dzcdn.net/images/cover/ab12/';
		expect(coverFor(`${dz}1000x1000-000000-80-0-0.jpg`, 464, 1).src).toBe(`${dz}500x500-000000-80-0-0.jpg`);
		expect(coverFor('/api/v1/music-trending/art/artist/abc', 200)).toEqual({ src: '/api/v1/music-trending/art/artist/abc' });
		expect(coverFor('/api/v1/music-trending/art/artist/abc', 44, 2).src).toBe('/api/v1/music-trending/art/artist/abc?size=250');
		expect(coverFor('/api/v1/artwork/ab12.jpg', 32, 2).src).toBe('/api/v1/artwork/ab12.jpg?size=250');
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
	});
});

describe('artist facts', () => {
	it('names the kind of act in our words', () => {
		expect(artistKind('Person')).toBe('Solo artist');
		expect(artistKind('group')).toBe('Group');
		expect(artistKind('Character')).toBe('Artist');
		expect(artistKind(undefined)).toBe('');
	});
	it('spells out the country', () => {
		expect(countryName('CA')).toBe('Canada');
		expect(countryName('gb')).toBe('United Kingdom');
		expect(countryName('XW')).not.toBe('');
		expect(countryName('')).toBe('');
	});
});
