import { groupDates } from './calendar-groups.ts';
import { cardLines, rowWords, channelShown, clockTime, episodeWords, fakeEpisodeName, movieCaption, musicCaption, ordinal, runLength, seriesLine, whereOut, type CaptionLook } from './calendar-captions.ts';
import { describe, expect, it } from 'vitest';
import type { CalendarEntry } from '../core/types.ts';

const look = (o: Partial<CaptionLook> = {}): CaptionLook => ({ movie: 6, music: 5, channels: 'all', region: 'US', ...o });
const ep = (o: Partial<CalendarEntry> = {}): CalendarEntry => ({
	title_id: 1, name: 'Grey', kind: 'episode', date: new Date(2026, 9, 29, 19, 0).toISOString(), timed: true, season: 24, episode: 25, network: 'ABC', item_id: 9, ...o
});
const odyssey: CalendarEntry = {
	title_id: 439, name: 'The Odyssey', year: 2026, kind: 'cinema', date: '2026-07-17T00:00:00Z', genres: ['Adventure', 'Action'], runtime: 170,
	directors: ['Christopher Nolan'], cast: ['Matt Damon', 'Tom Holland'], home_date: '2026-11-17T00:00:00Z'
};
const album = (o: Partial<CalendarEntry> = {}): CalendarEntry => ({
	title_id: 5, name: 'Remi Wolf', kind: 'album', date: '2026-10-09T00:00:00Z', part_id: 3, album: 'Big Ideas', album_type: 'album', tracks: 12, length_min: 44, nth: 3, ...o
});

describe('episode captions', () => {
	it('hides names that only repeat the number or the day', () => {
		for (const n of ['Episode 8827', 'episode 4', '19/10/2026', '2026-10-19', '02/11/2026', 'October 19, 2026', '19 October 2026', 'TBA', '', undefined]) expect(fakeEpisodeName(n)).toBe(true);
		for (const n of ['Collide', 'The Day After', 'Part of the Plan']) expect(fakeEpisodeName(n)).toBe(false);
	});
	it('says the time in the viewer\'s own format', () => {
		expect(clockTime(ep(), 'en-US')).toBe('7 PM');
		expect(clockTime(ep({ date: new Date(2026, 9, 29, 19, 30).toISOString() }), 'en-US')).toBe('7:30 PM');
		expect(clockTime(ep(), 'en-GB')).toBe('19:00');
		expect(clockTime(ep({ timed: false }), 'en-US')).toBe('');
	});
	it('lists the name (real ones only), the season line and the channel, with the time in its column', () => {
		const w = rowWords(groupDates([ep({ episode_name: 'Collide' })])[0], look(), '2026-10-27');
		expect(w.title).toBe('Grey');
		expect(w.lines).toEqual(['Collide', 'Season 24, Episode 25', 'ABC']);
		expect(w.when).toBeTruthy();
		const f = rowWords(groupDates([ep({ episode_name: 'Episode 25', timed: false })])[0], look({ channels: 'known', region: 'ZA' }), '2026-10-27');
		expect(f.lines).toEqual(['Season 24, Episode 25']);
		expect([f.when, f.quiet]).toEqual(['All day', true]);
		expect(cardLines(groupDates([ep({ episode_name: '02/11/2026' })])[0], look(), '2026-10-27').slice(1)).toEqual(['Season 24, Episode 25']);
	});
	it('spans several episodes of one day', () => {
		expect(episodeWords([{ season: 39, episode: 196 }, { season: 39, episode: 194 }])).toBe('Season 39, Episodes 194–196');
	});
	it('names known channels only with known', () => {
		expect(channelShown('M-Net', look())).toBe(true);
		expect(channelShown('M-Net', look({ channels: 'known' }))).toBe(false);
		expect(channelShown('M-Net', look({ channels: 'known', region: 'ZA' }))).toBe(true);
		expect(channelShown('Netflix', look({ channels: 'known', region: 'global' }))).toBe(true);
		expect(channelShown('CBS', look({ channels: 'known' }))).toBe(true);
		expect(channelShown('BBC One', look({ channels: 'known' }))).toBe(false);
	});
});

describe('movie captions', () => {
	it('has six choices', () => {
		expect(movieCaption(odyssey, 1)).toBe('Adventure · 2h 50m');
		expect(movieCaption(odyssey, 2)).toBe('Christopher Nolan');
		expect(movieCaption(odyssey, 3)).toMatch(/^In cinemas · At home from /);
		expect(movieCaption(odyssey, 4)).toBe('Matt Damon · Adventure');
		expect(movieCaption(odyssey, 6)).toBe('Christopher Nolan · 2h 50m');
		expect(movieCaption(odyssey, 5)).toBe('Adventure · 2h 50m');
	});
	it('names the series and the place the name gives', () => {
		expect(seriesLine({ name: 'Dune: Part Three', collection: 'Dune Collection' })).toBe('Dune · Part 3');
		expect(seriesLine({ name: 'Toy Story 4', collection: 'Toy Story Collection' })).toBe('Toy Story · 4th film');
		expect(seriesLine({ name: 'Avengers: Doomsday', collection: 'The Avengers Collection' })).toBe('The Avengers series');
		expect(seriesLine({ name: 'Solo' })).toBe('');
	});
	it('lists the pick, the genre and the studio; the column says Cinema, At home or Out now', () => {
		const w = rowWords(groupDates([{ ...odyssey, studio: 'Syncopy' }])[0], look(), '2026-10-27');
		expect(w).toEqual({ title: 'The Odyssey', lines: ['Christopher Nolan · 2h 50m', 'Adventure', 'Syncopy'], when: 'In theaters', quiet: false });
		expect(rowWords(groupDates([{ ...odyssey, kind: 'digital', date: '2026-11-17T00:00:00Z' }])[0], look(), '2026-10-27').when).toBe('Streaming');
		expect(rowWords(groupDates([{ ...odyssey, kind: 'digital', date: '2026-10-01T00:00:00Z' }])[0], look({ movie: 1 }), '2026-10-27')).toMatchObject({ when: 'Streaming', lines: ['Adventure · 2h 50m'] });
	});
	it('says where it is out', () => {
		expect(whereOut({ ...odyssey, home_date: undefined })).toBe('In cinemas');
		expect(whereOut({ ...odyssey, kind: 'digital', date: '2026-11-17T00:00:00Z', cinema_date: '2026-07-17T00:00:00Z' })).toMatch(/^At home · In cinemas /);
	});
	it('reads short lengths and ordinals', () => {
		expect(runLength(120)).toBe('2h');
		expect(runLength(45)).toBe('45m');
		expect([1, 2, 3, 4, 11, 12, 13, 21, 22].map(ordinal).join(' ')).toBe('1st 2nd 3rd 4th 11th 12th 13th 21st 22nd');
	});
});

describe('music captions', () => {
	it('has five choices', () => {
		expect(musicCaption(album(), 1)).toBe('Album · 12 songs');
		expect(musicCaption(album(), 2)).toBe('Album · 44 min');
		expect(musicCaption(album(), 3)).toBe('Album · 12 songs');
		expect(musicCaption(album({ genres: ['pop'] }), 4)).toBe('Pop album · 12 songs');
		expect(musicCaption(album(), 4)).toBe('Album · 12 songs');
		expect(musicCaption(album(), 5)).toBe('3rd album · 12 songs');
		expect(musicCaption(album({ nth: 1 }), 5)).toBe('Debut album · 12 songs');
		expect(musicCaption(album({ album_type: 'single', tracks: 1 }), 5)).toBe('New single');
	});
	it('puts the artist first, then the album and the pick', () => {
		expect(rowWords(groupDates([album()])[0], look(), '2026-10-27')).toEqual({ title: 'Remi Wolf', lines: ['Big Ideas', '3rd album · 12 songs'], when: 'Album', quiet: false });
		expect(rowWords(groupDates([album({ date: '2026-11-20T00:00:00Z', album_type: 'ep' })])[0], look(), '2026-10-27').when).toBe('EP');
		expect(cardLines(groupDates([album()])[0], look(), '2026-10-27')).toEqual(['Album', 'Remi Wolf', '3rd album · 12 songs']);
	});
});

