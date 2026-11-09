import { describe, expect, it } from 'vitest';
import { choices, deleteSummary, episodeFileLine, matchKey, showFolderLine, showIdOf, startPicks, type AdoptCandidate, type RemovePlan } from './organise.ts';

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
		expect(deleteSummary({ ...base, scope: 'title' }, 'show')[0]).toMatch(/^Leaves your Library/);
		const season = deleteSummary({ ...base, scope: 'season', season: 2, episodes: ['S02E01', 'S02E02'] }, 'show');
		expect(season[0]).toBe('Season 2: 2 episodes stop being kept up to date');
		const one = deleteSummary({ ...base, scope: 'episodes', episodes: ['S01E02'], kept: [{ file_id: 3, origin: 'adopted', path: 'x', why: 'kept: it also holds S01E03, which stays' }] }, 'show');
		expect(one[0]).toMatch(/^S01E02: no longer kept up to date/);
		expect(one).toContain('1 file stays on disk');
		expect(deleteSummary(base)[0]).toMatch(/^Leaves your Library/);
	});
});
