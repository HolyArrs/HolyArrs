import { describe, expect, it } from 'vitest';
import { choices, deleteSummary, episodeFileLine, matchKey, showFolderLine, showIdOf, startPicks, type AdoptCandidate, type RemovePlan } from './organise.ts';
import { byDay, entryDay, entryName, entryTime, shows } from './calendar.ts';
import { filterParams, historyQuery, readFilter } from './history.ts';
import type { CalendarEntry } from './types.ts';

describe('show library import', () => {
	const show: AdoptCandidate = {
		folder: 'Copper Valley (2025)',
		parsed_name: 'Copper Valley',
		videos: [],
		match: { id: 'tvdb:200', tmdb_id: '100', name: 'Copper Valley', year: 2025, score: 1 },
		others: [],
		status: 'matched',
		why: '',
		files: [
			{ path: 'a/S01E01.mkv', size: 1, episodes: ['S01E01'] },
			{ path: 'a/S01E02E03.mkv', size: 1, episodes: ['S01E02', 'S01E03'] },
			{ path: 'a/extra.mkv', size: 1, episodes: [], why: 'the file name gives no episode numbers' }
		],
		episodes: 3,
		unknown: 1
	};
	it('adopts a show folder by its show id, a movie folder by TMDB', () => {
		expect(matchKey(show.match!)).toBe('tvdb:200');
		expect(matchKey({ tmdb_id: '693134', name: 'Dune', year: 2024, score: 1 })).toBe('693134');
		expect(startPicks([show])).toEqual({ 'Copper Valley (2025)': 'tvdb:200' });
		expect(choices({ 'Copper Valley (2025)': 'tvdb:200', Empty: '' }, 'show')).toEqual([{ folder: 'Copper Valley (2025)', id: 'tvdb:200' }]);
		expect(choices({ Dune: '693134' })).toEqual([{ folder: 'Dune', tmdb_id: '693134' }]);
		expect(showIdOf({ source: 'tmdb', ids: { tmdb: '100', tvdb: '200' } })).toBe('tmdb:100');
		expect(showIdOf({ source: 'tvdb', ids: { tmdb: '100' } })).toBe('tmdb:100');
		expect(showIdOf({ source: 'tvdb', ids: {} })).toBe('');
	});
	it('says what each file holds and sums a folder up', () => {
		expect(episodeFileLine(show.files![1])).toBe('S01E02, S01E03');
		expect(episodeFileLine(show.files![2])).toBe('Left alone: the file name gives no episode numbers');
		expect(showFolderLine(show)).toBe('3 episodes in 2 files; 1 file with no episode, left alone');
		expect(showFolderLine({ episodes: 1, unknown: 0, files: [show.files![0]] })).toBe('1 episode in 1 file');
	});
});

describe('show deletes', () => {
	const base: RemovePlan = { title_id: 1, name: 'Copper Valley', choice: { delete_files: true, never_add: false }, applied: false, held: [], kept: [], holding_days: 7, active_downloads: 0 };
	it('words a whole show, a season and episodes', () => {
		expect(deleteSummary({ ...base, scope: 'title' }, 'show')[0]).toMatch(/^The show leaves the Library/);
		const season = deleteSummary({ ...base, scope: 'season', season: 2, episodes: ['S02E01', 'S02E02'] }, 'show');
		expect(season[0]).toBe('Season 2 (2 episodes): HolyArrs stops keeping them up to date. The show stays, and you can turn Keep up to date on again later.');
		const one = deleteSummary({ ...base, scope: 'episodes', episodes: ['S01E02'], kept: [{ file_id: 3, origin: 'adopted', path: 'x', why: 'kept: it also holds S01E03, which stays' }] }, 'show');
		expect(one[0]).toMatch(/^S01E02: HolyArrs stops keeping it up to date/);
		expect(one).toContain('1 file stays where it is.');
		expect(deleteSummary(base)[0]).toMatch(/^The movie leaves/);
	});
});

describe('episodes on the calendar', () => {
	const ep = (over: Partial<CalendarEntry>): CalendarEntry => ({ title_id: 3, name: 'Copper Valley', kind: 'episode', date: '2026-10-12T02:00:00Z', season: 1, episode: 2, item_id: 9, ...over });
	it('puts a timed episode on the local day and time', () => {
		const e = ep({ timed: true, air_day: '2026-10-11', episode_name: 'The Mill' });
		const local = new Date('2026-10-12T02:00:00Z');
		const p = (n: number) => String(n).padStart(2, '0');
		expect(entryDay(e)).toBe(`${local.getFullYear()}-${p(local.getMonth() + 1)}-${p(local.getDate())}`);
		expect(entryTime(e)).toBe(`${p(local.getHours())}:${p(local.getMinutes())}`);
		expect(entryName(e)).toBe('Copper Valley S01E02 The Mill');
	});
	it('uses the viewer time zone, not the UTC date or the air day', () => {
		const was = process.env.TZ;
		process.env.TZ = 'America/New_York';
		try {
			const e = ep({ timed: true, date: '2026-10-12T02:00:00Z', air_day: '2026-10-12' });
			expect(entryDay(e)).toBe('2026-10-11');
			expect(entryTime(e)).toBe('22:00');
			process.env.TZ = 'Asia/Tokyo';
			expect(entryDay(ep({ timed: true, date: '2026-10-12T20:00:00Z', air_day: '2026-10-12' }))).toBe('2026-10-13');
		} finally {
			if (was === undefined) delete process.env.TZ;
			else process.env.TZ = was;
		}
	});
	it('puts an episode without a time on its air day', () => {
		const e = ep({ timed: false, date: '2026-10-20T00:00:00Z', air_day: '2026-10-20' });
		expect(entryDay(e)).toBe('2026-10-20');
		expect(entryTime(e)).toBe('');
		expect(entryName({ title_id: 1, name: 'Dune', year: 2024, kind: 'digital', date: '2024-04-16T00:00:00Z' })).toBe('Dune (2024)');
	});
	it('filters movies and shows and keeps days in order', () => {
		const m: CalendarEntry = { title_id: 1, name: 'Dune', kind: 'cinema', date: '2026-10-21T00:00:00Z' };
		const e = ep({ date: '2026-10-20T00:00:00Z', air_day: '2026-10-20' });
		expect(shows(m, 'movies') && !shows(e, 'movies')).toBe(true);
		expect(shows(e, 'shows') && !shows(m, 'shows')).toBe(true);
		expect(shows(m, 'all') && shows(e, 'all')).toBe(true);
		expect(byDay([m, e]).map((d) => d.day)).toEqual(['2026-10-20', '2026-10-21']);
	});
});

describe('history of one episode', () => {
	it('asks for the episode and keeps it in the address', () => {
		const f = readFilter(new URLSearchParams('title=3&episode=41'));
		expect(f.episode).toBe(41);
		expect(new URLSearchParams(historyQuery(f)).get('episode')).toBe('41');
		expect(new URLSearchParams(historyQuery(f)).get('title')).toBeNull();
		expect(filterParams(f)).toBe('?title=3&episode=41');
		expect(readFilter(new URLSearchParams('episode=x')).episode).toBeUndefined();
	});
});
