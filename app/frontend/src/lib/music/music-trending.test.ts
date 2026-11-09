import { fetchMusicExplore, fetchMusicExploreMore, listeners, releaseWords, musicCardHref, musicCardNote, type MusicTrendAnswer } from './music-trending.ts';
import { describe, expect, it } from 'vitest';

const bts = '0d79fe8e-ba27-4859-bb8c-2f255f346853';
const arirang = '21159e3f-172e-43f6-aa7d-8e06a81fea49';

const albums: MusicTrendAnswer = {
	kind: 'album',
	region: 'global',
	source: 'listenbrainz',
	regions: ['global', 'FR'],
	items: [
		{ rank: 1, mbid: arirang, name: 'ARIRANG', artist_name: 'BTS', artist_mbid: bts },
		{ rank: 2, mbid: '66666666-6666-4666-8666-666666666666', name: 'No artist id', artist_name: 'Someone' }
	]
};

describe('music pass', () => {
	it('shapes and listens', () => {
		expect(listeners(1500)).toBe('1.5K listens this week');
		expect(listeners(0)).toBe('');
	});
});

describe('Explore music cards', () => {
	const base = { key: 'k', mbid: arirang, name: 'ARIRANG', genres: [] };
	it('says why each card is there, by row', () => {
		expect(musicCardNote({ type: 'new_from' }, { ...base, kind: 'album', artist: 'BTS', date: new Date(Date.now() - 3 * 86400000).toISOString().slice(0, 10) })).toBe('BTS · Out now');
		expect(musicCardNote({ type: 'new_from' }, { ...base, kind: 'album', artist: 'BTS', date: new Date(Date.now() + 40 * 86400000).toISOString().slice(0, 10), upcoming: true })).toMatch(/^BTS · Out (?!now)/);
		expect(musicCardNote({ type: 'just_arrived' }, { ...base, kind: 'album', artist: 'BTS', date: '2026-10-18' })).toMatch(/Arrived /);
		expect(musicCardNote({ type: 'your_asks' }, { ...base, kind: 'artist', state: 'approved' })).toBe('Approved, on the way');
		expect(musicCardNote({ type: 'top_albums' }, { ...base, kind: 'album', artist: 'BTS', listen_count: 12000 })).toBe('BTS · 12K listens this week');
		expect(musicCardNote({ type: 'missing' }, { ...base, kind: 'album', artist: 'BTS', date: '1997-05-21' })).toBe('BTS · 1997');
		expect(musicCardNote({ type: 'because_have' }, { ...base, kind: 'artist' })).toBe('');
		expect(musicCardNote({ type: 'collabs' }, { ...base, kind: 'artist', note: 'With Radiohead' })).toBe('With Radiohead');
		expect(musicCardNote({ type: 'classic' }, { ...base, kind: 'album', artist: 'Radiohead', date: '1997-05-21' })).toBe('Radiohead · 1997');
	});

	it('opens the album and artist pages, else Library pages or the artist search', () => {
		expect(musicCardHref({ ...base, kind: 'album', mbid: arirang, artist_mbid: bts })).toBe(`/music/album/${arirang}?artist=${bts}`);
		expect(musicCardHref({ ...base, kind: 'artist', mbid: bts, title_id: 4 })).toBe(`/music/artist/${bts}`);
		expect(musicCardHref({ ...base, kind: 'album', mbid: '', title_id: 4, part_id: 9 })).toMatch(/\/library\/4\/albums\/9$/);
		expect(musicCardHref({ ...base, mbid: '', kind: 'artist', title_id: 4 })).toMatch(/\/library\/4$/);
		expect(musicCardHref({ ...base, mbid: '', kind: 'album', artist: 'BTS' })).toMatch(/\/search\?q=BTS&kind=music$/);
	});

	it('reads the rows for a chip and region', async () => {
		let asked = '';
		const client = { get: async <T,>(p: string) => ((asked = p), { kid: false, cold: true, genres: [], genre: '', hero: null, rows: [] } as T) };
		await fetchMusicExplore('rock', 'FR', client, undefined, 21);
		expect(asked).toBe('/explore/music?region=FR&hour=21&genre=rock');
	});

	it('reads the next rows with the cursor (music_rows_v7)', async () => {
		let asked = '';
		const client = { get: async <T,>(p: string) => ((asked = p), { kid: false, cold: false, genres: [], genre: '', hero: null, rows: [], next: '' } as T) };
		await fetchMusicExploreMore('abc.def', 'US', client);
		expect(asked).toBe('/explore/music?region=US&cursor=abc.def');
	});
});

describe('releaseWords', () => {
	const now = new Date(2026, 9, 20, 15, 0);
	it('says Out with the day for an album still to come', () => {
		expect(releaseWords('2026-10-25', now)).toMatch(/^Out .*25/);
		expect(releaseWords('2026-10-21', now)).toMatch(/^Out .*21/);
	});
	it('says Out now from release day to 14 days after', () => {
		expect(releaseWords('2026-10-20', now)).toBe('Out now');
		expect(releaseWords('2026-10-06', now)).toBe('Out now');
	});
	it('says Released with the day after that', () => {
		expect(releaseWords('2026-10-05', now)).toMatch(/^Released .*5/);
		expect(releaseWords('2026-09-25', now)).toMatch(/^Released .*25/);
	});
	it('says nothing for a bad date', () => {
		expect(releaseWords('soon', now)).toBe('');
	});
});
