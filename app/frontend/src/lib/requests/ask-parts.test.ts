import { albumCounts, albumGroups, askAlbums, askSeasons, askedWords, songsHead, picksChanged, picksHead, seasonPick, baseName, keptByDefault, keptSeasons, keptWords, seasonsToAsk, kindOf, type ArtistAlbum } from './ask-parts.ts';
import { describe, expect, it } from 'vitest';

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
		expect(albumCounts(albumGroups([al('g', 'Live at Lowlands, 2015', '2016'), al('h', 'Night (Live)', '2017')]))).toBe('2 live albums');
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

describe('episode ticks', () => {
	const rows = askSeasons({ seasons: [1], episodes: [{ season: 2, episodes: [5, 1, 2, 2, 3, 4] }] }, [{ number: 1, episode_count: 8 }, { number: 2, episode_count: 8 }, { number: 3, episode_count: 0 }]);
	it('marks a partly asked season', () => {
		expect(rows.map((r) => r.asked)).toEqual([undefined, [1, 2, 3, 4, 5]]);
		const [one, two] = rows.map((r) => seasonPick(r, null));
		expect([one.tick, two.tick]).toEqual(['all', 'some']);
		expect(askedWords(one)).toBe('all 8 episodes');
		expect(askedWords(two)).toBe('5 of 8 episodes');
		expect(picksHead([one, two])).toBe('1 season + 5 episodes · 13 episodes');
		expect(picksChanged([one, two])).toBe(false);
	});
	it('counts a season from its episode list when the count is missing', () => {
		const p = seasonPick({ number: 3, episodes: 0 }, [{ number: 1 }, { number: 2 }]);
		expect(p.all).toEqual([1, 2]);
		expect(askedWords(p)).toBe('all 2 episodes');
	});
	it('adds an un-asked episode and drops an asked one', () => {
		const add = seasonPick(rows[1], null, [1, 2, 3, 4, 5, 6]);
		const drop = seasonPick(rows[0], null, [1, 2, 3, 4, 5, 6, 7]);
		expect(picksChanged([drop, add])).toBe(true);
		expect(picksHead([drop, add])).toBe('13 episodes from 2 seasons');
		expect(picksHead([seasonPick(rows[0], null, []), seasonPick(rows[1], null, [])]).startsWith('0 of 2')).toBe(true);
	});
});
describe('song ticks', () => {
	it('adds song totals', () => {
		expect(songsHead('6 albums', [{ all: 12, ticked: 12 }, { all: 10, ticked: 10 }])).toBe('6 albums · 22 songs');
		expect(songsHead('6 albums', [{ all: 12, ticked: 9 }, { all: null, ticked: 0 }])).toBe('6 albums · 9 of 12 songs');
		expect(songsHead('6 albums', [{ all: null, ticked: 0 }])).toBe('6 albums');
	});
});

describe('bootleg names (#135)', () => {
	it('a bootleg on the albums shelf is not an album', () => {
		expect(kindOf({ type: 'album', secondary_types: [], shelf: 'albums', name: 'Unreleased Demos 2009' })).toBe('other');
		expect(kindOf({ type: 'album', secondary_types: [], shelf: 'albums', name: 'Views' })).toBe('albums');
	});
});

describe('seasons a Library show keeps', () => {
	const owned = (keep: boolean, seasons: [number, boolean][]) => ({ keep_up_to_date: keep, show: { seasons: seasons.map(([number, keep_up_to_date]) => ({ number, keep_up_to_date })) } });
	it('lists the seasons whose own switch and the show switch are on', () => {
		expect(keptSeasons(owned(true, [[0, false], [1, false], [2, true], [3, false]]))).toEqual([2]);
		expect(keptSeasons(owned(false, [[1, true], [2, true]]))).toEqual([]);
		expect(keptSeasons({ keep_up_to_date: true })).toEqual([]);
	});
	it('finds the regular seasons left to ask for', () => {
		const all = [{ number: 0 }, { number: 1 }, { number: 2 }, { number: 3 }];
		expect(seasonsToAsk(all, [2])).toEqual([1, 3]);
		expect(seasonsToAsk(all, [1, 2, 3])).toEqual([]);
		expect(seasonsToAsk(undefined, [])).toEqual([]);
	});
});
