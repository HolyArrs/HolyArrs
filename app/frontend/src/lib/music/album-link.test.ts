import { albumArtistFix, musicLoadError } from './music-pages.ts';
import { describe, expect, it } from 'vitest';

const A = 'b10bbbfc-cf9e-42e0-be17-e2c3e1d2600d';
const B = '5b11f4ce-a62d-471e-81fc-a69a8278c7da';

describe('albumArtistFix', () => {
	it('mends a wrong artist in the link with the album\'s own artist', () => {
		expect(albumArtistFix(A, B)).toBe(B);
		expect(albumArtistFix('', B)).toBe(B);
	});
	it('leaves a right link alone', () => {
		expect(albumArtistFix(A, A.toUpperCase())).toBe('');
		expect(albumArtistFix(A, undefined)).toBe('');
	});
});

describe('musicLoadError', () => {
	it('says "check the connection" only for a real network failure', () => {
		expect(musicLoadError({ status: 0 }, 'album').network).toBe(true);
		for (const status of [400, 404, 502, 503]) expect(musicLoadError({ status }, 'album').network).toBe(false);
	});
	it('uses calm sentences, never the backend text', () => {
		expect(musicLoadError({ status: 404, message: 'that album is not by that artist' }, 'album').text).toBe('We could not find this album.');
		expect(musicLoadError(new Error('boom'), 'artist').text).toBe('This artist is taking longer than usual to load.');
	});
});
