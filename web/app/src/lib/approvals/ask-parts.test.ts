import { describe, expect, it } from 'vitest';
import { albumCounts, albumGroups, askAlbums, askSeasons, baseName, keptByDefault, keptWords, type ArtistAlbum } from './ask-parts.ts';

const al = (mbid: string, name: string, date: string, extra: Partial<ArtistAlbum> = {}): ArtistAlbum => ({ mbid, name, type: 'album', secondary_types: [], release_date: date, shelf: 'albums', image: `/art/${mbid}`, ...extra });

describe('album rows', () => {
	it('drops edition tags from a name', () => {
		expect(baseName('Harbour Lights (Deluxe Edition)')).toBe('harbour lights');
		expect(baseName('Harbour Lights - 2011 Remaster')).toBe('harbour lights');
		expect(baseName('Harbour Lights [10th Anniversary] (Explicit)')).toBe('harbour lights');
		expect(baseName('Songs (for Ana)')).toBe('songs (for ana)');
		expect(baseName('Night - Live')).toBe('night - live');
	});
	it('folds editions under the main album and counts each album once', () => {
		const rows = albumGroups([
			al('b', 'Harbour Lights (Deluxe)', '2013-01-01'),
			al('a', 'Harbour Lights', '2012-05-01'),
			al('c', 'Harbour Lights (10th Anniversary Edition)', '2022-05-01'),
			al('d', 'Second Wind', '2015'),
			al('e', 'Wind EP', '2014', { type: 'ep', shelf: 'singles' }),
			al('f', 'Live Night', '2016', { secondary_types: ['live'], shelf: 'live' })
		]);
		expect(rows.map((r) => r.main.name)).toEqual(['Harbour Lights', 'Second Wind', 'Wind EP', 'Live Night']);
		expect(rows[0].ids).toEqual(['a', 'b', 'c']);
		expect(rows[0].editions.map((e) => e.year)).toEqual([2013, 2022]);
		expect(albumCounts(rows)).toBe('2 albums · 1 single or EP · 1 live album');
	});
	it('takes what the music defaults take for a whole-artist ask', () => {
		expect(keptByDefault({ type: 'album', secondary_types: [] })).toBe(true);
		expect(keptByDefault({ type: 'ep', secondary_types: [] })).toBe(true);
		expect(keptByDefault({ type: 'single', secondary_types: [] })).toBe(false);
		expect(keptByDefault({ type: 'album', secondary_types: ['live'] })).toBe(false);
		expect(keptByDefault({ type: 'album', secondary_types: [], upcoming: true })).toBe(false);
		const artist = [al('a', 'One', '2010'), al('l', 'Live', '2011', { secondary_types: ['live'], shelf: 'live' })];
		expect(askAlbums({ albums: [] }, artist).map((x) => x.mbid)).toEqual(['a']);
		expect(askAlbums({ albums: [{ mbid: 'l', name: 'Live' }, { mbid: 'z', name: 'Gone', year: 2001 }] }, artist).map((x) => x.name)).toEqual(['Live', 'Gone']);
		expect(askAlbums({ albums: [] }, null)).toEqual([]);
	});
});

describe('season rows', () => {
	const seasons = [{ number: 0, episode_count: 3 }, { number: 1, episode_count: 10, year: 2020 }, { number: 2, episode_count: 8 }];
	it('lists the asked seasons, or every regular season', () => {
		expect(askSeasons({ seasons: [2] }, seasons)).toEqual([{ number: 2, episodes: 8, year: undefined }]);
		expect(askSeasons({ seasons: [] }, seasons).map((s) => s.number)).toEqual([1, 2]);
		expect(askSeasons({ seasons: [], future_only: true }, seasons)).toEqual([]);
	});
	it('says how much is kept', () => {
		expect(keptWords(2, 2, 'season', '18 episodes')).toBe('All 2 seasons · 18 episodes');
		expect(keptWords(1, 2, 'season')).toBe('1 of 2 seasons');
		expect(keptWords(1, 1, 'album')).toBe('1 album');
	});
});
