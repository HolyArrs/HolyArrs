import { describe, expect, it } from 'vitest';
import {
	airDateChoices,
	multiEpisodeChoices,
	episodeExampleRows,
	showWords,
	canAdopt,
	choices,
	deleteSummary,
	fileName,
	finished,
	followJob,
	groupPlan,
	score,
	splitSteps,
	startPicks,
	type AdoptCandidate,
	type JobView,
	type RemovePlan
} from './organise.ts';

const cand = (folder: string, status: AdoptCandidate['status'], tmdb = ''): AdoptCandidate => ({
	folder,
	parsed_name: folder,
	videos: [],
	others: [],
	status,
	why: '',
	match: tmdb ? { tmdb_id: tmdb, name: folder, year: 2020, score: 0.95 } : undefined
});

describe('rename preview', () => {
	it('splits changes, skips and files already named right', () => {
		const r = splitSteps([
			{ file_id: 1, title_id: 1, title_name: 'A', origin: 'imported', from: 'a/x.mkv', to: 'a/A (2020).mkv' },
			{ file_id: 2, title_id: 1, title_name: 'A', origin: 'found', from: 'a/y.mkv', skip: 'found files are never renamed' },
			{ file_id: 3, title_id: 2, title_name: 'B', origin: 'adopted', from: 'b/B.mkv', to: 'b/B.mkv' }
		]);
		expect(r.change.map((s) => s.file_id)).toEqual([1]);
		expect(r.skipped.map((s) => s.file_id)).toEqual([2]);
		expect(r.same).toBe(1);
	});
	it('takes the file name from a path', () => {
		expect(fileName('a/b/c.mkv')).toBe('c.mkv');
		expect(fileName('a\\c.mkv')).toBe('c.mkv');
		expect(fileName('c.mkv')).toBe('c.mkv');
	});
});

describe('adopt plan', () => {
	const folders = [cand('M1', 'matched', '1'), cand('U1', 'unsure', '2'), cand('X', 'unmatched'), cand('L', 'in_library'), cand('N', 'no_video')];
	it('groups in a fixed order and leaves out empty groups', () => {
		expect(groupPlan(folders).map((g) => g.status)).toEqual(['matched', 'unsure', 'unmatched', 'in_library', 'no_video']);
	});
	it('only offers folders that can be adopted', () => {
		expect(folders.filter(canAdopt).map((f) => f.folder)).toEqual(['M1', 'U1', 'X']);
	});
	it('ticks only the matched folders at first', () => {
		expect(startPicks(folders)).toEqual({ M1: '1' });
	});
	it('drops picks without a movie', () => {
		expect(choices({ M1: '1', X: '' })).toEqual([{ folder: 'M1', tmdb_id: '1' }]);
	});
	it('shows a score as a percent', () => {
		expect(score(0.856)).toBe('86%');
		expect(score(2)).toBe('100%');
	});
});

describe('delete plan', () => {
	const base: RemovePlan = { title_id: 1, name: 'A', choice: { delete_files: false, never_add: false }, applied: false, held: [], kept: [], holding_days: 7, active_downloads: 0 };
	it('says files stay by default', () => {
		const s = deleteSummary({ ...base, kept: [{ file_id: 1, origin: 'imported', path: 'a.mkv', why: 'delete files is off' }] });
		expect(s.join(' ')).toContain('1 file stays on disk');
		expect(s.join(' ')).not.toContain('holding');
	});
	it('says held files go after the holding days', () => {
		const held = [1, 2].map((i) => ({ file_id: i, origin: 'imported', path: `${i}.mkv`, why: '' }));
		const s = deleteSummary({ ...base, choice: { delete_files: true, never_add: true }, held, active_downloads: 1 }).join(' ');
		expect(s).toContain('2 files to the holding folder for 7 days');
		expect(s).toContain('Lists never add it again');
		expect(s).toContain('1 download keep running');
	});
	it('says when holding days is 0', () => {
		const s = deleteSummary({ ...base, holding_days: 0, held: [{ file_id: 1, origin: 'adopted', path: 'a', why: '' }] }).join(' ');
		expect(s).toContain('1 file to the holding folder');
	});
	it('says when there is nothing on disk', () => {
		expect(deleteSummary(base).join(' ')).toContain('No files on disk');
	});
});

describe('following a Job', () => {
	it('knows when a Job has ended', () => {
		expect(finished({ State: 'done' })).toBe(true);
		expect(finished({ State: 'running' })).toBe(false);
	});
	it('reads until the Job ends and reports each read', async () => {
		const states: JobView['State'][] = ['queued', 'running', 'done'];
		let i = 0;
		const seen: number[] = [];
		const j = await followJob(
			async (id) => ({ ID: id, State: states[Math.min(i++, 2)], Progress: i / 3 }),
			7,
			(x) => seen.push(x.Progress),
			{ every: 1 }
		);
		expect(j.State).toBe('done');
		expect(j.ID).toBe(7);
		expect(seen.length).toBe(3);
	});
	it('reads again at once when poked', async () => {
		let wake = () => {};
		let n = 0;
		const p = followJob(async () => ({ ID: 1, State: n++ ? 'done' : 'running', Progress: 0 }), 1, () => setTimeout(() => wake(), 0), {
			every: 60_000,
			poke: (w) => {
				wake = w;
				return () => {};
			}
		});
		expect((await p).State).toBe('done');
	});
});

describe('episode name examples', () => {
	it('lists the sample episodes in order with plain labels', () => {
		expect(episodeExampleRows(undefined)).toEqual([]);
		expect(episodeExampleRows({ show_folder: 'X', anime: 'a', episode: 'e', multi: 'm' })).toEqual([
			['One episode', 'e'],
			['A file holding two episodes', 'm'],
			['A show ordered by running number', 'a']
		]);
		expect(showWords).toContain('episode');
		expect(showWords).not.toContain('title');
		expect(showWords).toContain('season_padded');
	});
	it('offers every multi-episode and air day style the server takes', () => {
		expect(multiEpisodeChoices.map(([v]) => v)).toEqual(['range', 'short_range', 'each', 'each_dash']);
		expect(airDateChoices.map(([v]) => v)).toEqual(['dash', 'dot', 'space']);
	});
});
