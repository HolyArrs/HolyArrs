import { genreHref, musicCardNote, musicStatus, songAskHref } from './music-trending.ts';
import { describe, expect, it } from 'vitest';

describe('music_rows_v2 helpers', () => {
	it('maps state to one chip', () => {
		expect(musicStatus({ state: 'have' })).toBe('in_library');
		expect(musicStatus({ state: 'asked' })).toBe('requested');
		expect(musicStatus({ state: 'approved' })).toBe('approved');
		expect(musicStatus({})).toBeUndefined();
	});
	it('sends song lines to their album through the artist', () => {
		expect(songAskHref({ artist_mbid: 'abc', name: 'Song' })).toContain('/music/artist/abc');
		expect(songAskHref({ artist: 'Band', name: 'Song' })).toContain('/search?q=Band%20Song&kind=music');
	});
	it('opens a genre tile on its Music genre page', () => {
		expect(genreHref({ name: 'Hip-Hop/Rap', kind: 'genre', mbid: 'hip-hop-rap' })).toContain('/music/genre/hip-hop-rap');
		expect(genreHref({ name: 'Hip-Hop/Rap' })).toContain('/search?q=Hip-Hop%2FRap&kind=music');
	});
	it('says when a coming album is out', () => {
		expect(musicCardNote({ type: 'coming_soon' }, { key: 'k', kind: 'album', mbid: 'm', name: 'N', artist: 'A', date: '2999-01-02', genres: [] })).toMatch(/^A · Out /);
	});
});
