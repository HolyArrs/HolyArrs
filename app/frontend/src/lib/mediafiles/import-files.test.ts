import { describe, expect, it } from 'vitest';
import { blocker, episodeCode, itemsFor, pickLine, resultLine, startPick, startTicked, type ByHandFile } from './import-files.ts';

const show = (over: Partial<ByHandFile> = {}): ByHandFile => ({
	path: '/dl/a/Copper.Valley.S01E02.mkv',
	name: 'a/Copper.Valley.S01E02.mkv',
	size: 10,
	kind: 'video',
	guess: { title_id: 7, title_name: 'Copper Valley', title_kind: 'show', year: 2025, version_id: 3, season: 1, episode: 2, confidence: 1 },
	...over
});

describe('import files by hand', () => {
	it('starts from the guess', () => {
		const p = startPick(show());
		expect(pickLine(p)).toBe('Copper Valley (2025) · S01E02');
		expect(startPick(show({ guess: { confidence: 0 } }))).toBeNull();
		expect(pickLine(null)).toBe('No match yet');
	});

	it('names episodes', () => {
		expect(episodeCode(1, 2)).toBe('S01E02');
		expect(episodeCode(undefined, 2)).toBe('');
	});

	it('says why a file cannot go', () => {
		const f = show();
		expect(blocker(f, startPick(f))).toBe('');
		expect(blocker(f, null)).toMatch(/Pick the movie/);
		expect(blocker(show({ kind: 'audio', note: 'music is not imported by hand yet' }), null)).toMatch(/music/);
		expect(blocker(f, { ...startPick(f)!, episode: undefined })).toMatch(/season and episode/);
		expect(blocker(f, { ...startPick(f)!, has_file: true })).toMatch(/already has a file/);
		expect(blocker(f, { ...startPick(f)!, has_file: true, replace: true })).toBe('');
		expect(blocker(show({ in_library: true }), startPick(f))).toMatch(/Library/);
	});

	it('ticks only sure guesses', () => {
		const f = show();
		expect(startTicked(f, startPick(f))).toBe(true);
		const unsure = show({ guess: { ...show().guess, confidence: 0.4 } });
		expect(startTicked(unsure, startPick(unsure))).toBe(false);
	});

	it('builds the request from ticked files', () => {
		const a = show();
		const b = show({ path: '/dl/movie.mkv', guess: { title_id: 9, title_name: 'Dune', title_kind: 'movie', confidence: 1 } });
		const c = show({ path: '/dl/c.mkv' });
		const picks = { [a.path]: startPick(a), [b.path]: startPick(b), [c.path]: null };
		const items = itemsFor([a, b, c], picks, { [a.path]: true, [b.path]: true, [c.path]: true });
		expect(items).toEqual([
			{ path: a.path, title_id: 7, version_id: 3, season: 1, episode: 2 },
			{ path: b.path, title_id: 9 }
		]);
	});

	it('sends replace only for a file that has one', () => {
		const a = show({ path: '/dl/movie.mkv', guess: { title_id: 9, title_name: 'Dune', title_kind: 'movie', confidence: 1, has_file: true } });
		const b = show({ path: '/dl/other.mkv', guess: { title_id: 8, title_name: 'Arrival', title_kind: 'movie', confidence: 1 } });
		const picks = { [a.path]: { ...startPick(a)!, replace: true }, [b.path]: { ...startPick(b)!, replace: true } };
		expect(itemsFor([a, b], picks, { [a.path]: true, [b.path]: true })).toEqual([
			{ path: a.path, title_id: 9, replace: true },
			{ path: b.path, title_id: 8 }
		]);
		expect(itemsFor([a], { [a.path]: startPick(a) }, { [a.path]: true })).toEqual([]);
	});

	it('words results', () => {
		expect(resultLine({ path: 'x', outcome: 'placed', placed_by: 'hardlink' })).toBe('Added to the Library (no extra space used)');
		expect(resultLine({ path: 'x', outcome: 'placed', placed_by: 'copy' })).toBe('Copied to the Library (uses extra space)');
		expect(resultLine({ path: 'x', outcome: 'refused', error: 'nope' })).toBe('Not imported: nope');
	});
});
