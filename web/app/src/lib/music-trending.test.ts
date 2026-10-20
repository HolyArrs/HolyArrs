import { describe, expect, it } from 'vitest';
import { ApiError } from './api.ts';
import { matchOwned, ownedIndex } from './explore.ts';
import { fetchMusicExplore, fetchMusicTrend, listeners, musicCardHref, musicCardNote, musicTrendCards, newFromYourArtists, type MusicTrendAnswer } from './music-trending.ts';
import type { TitleView } from './types.ts';

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

describe('musicTrendCards', () => {
	it('maps albums to cards that point at their artist, with a cached cover', () => {
		const cards = musicTrendCards(albums);
		expect(cards).toHaveLength(1);
		expect(cards[0]).toMatchObject({ key: `album:${arirang}`, kind: 'artist', name: 'ARIRANG', subtitle: 'BTS', ref: `mbid:${bts}`, rank: 1, artist: 'BTS', artistMbid: bts });
		expect(cards[0].image).toMatch(new RegExp(`/api/v1/music-trending/art/album/${arirang}$`));
	});

	it('maps artists to their own id and picture', () => {
		const [c] = musicTrendCards({ ...albums, kind: 'artist', items: [{ rank: 3, mbid: bts, name: 'BTS' }] });
		expect(c).toMatchObject({ key: `artist:${bts}`, name: 'BTS', subtitle: undefined, ref: `mbid:${bts}`, artist: 'BTS', rank: 3 });
		expect(c.image).toMatch(/\/api\/v1\/music-trending\/art\/artist\//);
	});

	it('finds an artist in the Library by MusicBrainz id', () => {
		const lib = [{ id: 9, kind: 'artist', name: 'BTS', artist: { mbid: bts }, slots: [] } as unknown as TitleView];
		const owned = matchOwned(musicTrendCards(albums)[0], ownedIndex(lib));
		expect(owned.titleId).toBe(9);
	});
});

describe('fetchMusicTrend', () => {
	it('falls back to global when the country has no list, and is null when nothing is there', async () => {
		const asked: string[] = [];
		const client = {
			get: async <T>(p: string): Promise<T> => {
				asked.push(p);
				if (p.includes('region=global')) return albums as T;
				throw new ApiError('none', 404);
			}
		};
		expect((await fetchMusicTrend('album', 'DE', client))?.region).toBe('global');
		expect(asked).toEqual(['/music-trending?kind=album&region=DE', '/music-trending?kind=album&region=global']);
		const none = { get: async <T>(): Promise<T> => ({ ...albums, items: [] }) as T };
		expect(await fetchMusicTrend('artist', 'global', none)).toBeNull();
		const down = { get: async <T>(): Promise<T> => Promise.reject(new ApiError('down', 502)) };
		expect(await fetchMusicTrend('artist', 'FR', down)).toBeNull();
	});
});

describe('music pass', () => {
	it('lists new albums from Library artists, newest first, last 60 days only', () => {
		const now = new Date('2026-10-19T12:00:00Z');
		const e = (d: string, album: string, part: number) => ({ title_id: 1, name: 'Artist', kind: 'album' as const, date: d, album, part_id: part });
		const got = newFromYourArtists([e('2026-10-01', 'Old-ish', 1), e('2026-10-15', 'Newest', 2), e('2026-06-01', 'Too old', 3), e('2026-11-01', 'Future', 4), e('2026-10-15', 'Newest', 2)], now);
		expect(got.map((a) => a.album)).toEqual(['Newest', 'Old-ish']);
	});
	it('shapes and listens', () => {
		const cards = musicTrendCards({ kind: 'album', region: 'global', source: 'x', regions: null, items: [{ rank: 1, mbid: 'rg', name: 'LP', artist_name: 'A', artist_mbid: 'am', listen_count: 12345 }] });
		expect(cards[0].shape).toBe('square');
		expect(cards[0].subtitle).toBe('A · 12K listens this week');
		expect(listeners(1500)).toBe('1.5K listens this week');
		expect(listeners(0)).toBe('');
	});
});

describe('Explore music cards', () => {
	const base = { key: 'k', mbid: arirang, name: 'ARIRANG', genres: [] };
	it('says why each card is there, by row', () => {
		expect(musicCardNote({ type: 'new_from' }, { ...base, kind: 'album', artist: 'BTS', date: '2026-10-09' })).toMatch(/^BTS · Out /);
		expect(musicCardNote({ type: 'new_from' }, { ...base, kind: 'album', artist: 'BTS', date: '2026-12-09', upcoming: true })).toMatch(/^BTS · Coming /);
		expect(musicCardNote({ type: 'just_arrived' }, { ...base, kind: 'album', artist: 'BTS', date: '2026-10-18' })).toMatch(/Arrived /);
		expect(musicCardNote({ type: 'your_asks' }, { ...base, kind: 'artist', state: 'approved' })).toBe('Approved, on the way');
		expect(musicCardNote({ type: 'top_albums' }, { ...base, kind: 'album', artist: 'BTS', listen_count: 12000 })).toBe('BTS · 12K listens this week');
		expect(musicCardNote({ type: 'missing' }, { ...base, kind: 'album', artist: 'BTS', date: '1997-05-21' })).toBe('BTS · 1997');
		expect(musicCardNote({ type: 'because_have' }, { ...base, kind: 'artist' })).toBe('');
	});

	it('opens Library albums and artists, else the artist search', () => {
		expect(musicCardHref({ ...base, kind: 'album', title_id: 4, part_id: 9 })).toMatch(/\/library\/4\/albums\/9$/);
		expect(musicCardHref({ ...base, kind: 'artist', title_id: 4 })).toMatch(/\/library\/4$/);
		expect(musicCardHref({ ...base, kind: 'album', artist: 'BTS' })).toMatch(/\/search\?q=BTS&kind=music$/);
	});

	it('reads the rows for a chip and region', async () => {
		let asked = '';
		const client = { get: async <T,>(p: string) => ((asked = p), { kid: false, cold: true, genres: [], genre: '', hero: null, rows: [] } as T) };
		await fetchMusicExplore('rock', 'FR', client);
		expect(asked).toBe('/explore/music?region=FR&genre=rock');
	});
});
