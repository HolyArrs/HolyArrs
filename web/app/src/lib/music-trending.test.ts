import { describe, expect, it } from 'vitest';
import { ApiError } from './api.ts';
import { matchOwned, ownedIndex } from './explore.ts';
import { fetchMusicTrend, musicTrendCards, type MusicTrendAnswer } from './music-trending.ts';
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
