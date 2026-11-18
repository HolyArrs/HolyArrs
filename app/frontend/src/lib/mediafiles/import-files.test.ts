import { afterEach, describe, expect, it } from 'vitest';
import en from '../core/i18n/messages/en/server-lines.json';
import { lang } from '../core/i18n/i18n.svelte.ts';
import { blocker, episodeCode, itemsFor, pickLine, resultLine, startPick, startTicked, oneTickPerTrack, type ByHandFile } from './import-files.ts';

const show = (over: Partial<ByHandFile> = {}): ByHandFile => ({
	path: '/dl/a/Copper.Valley.S01E02.mkv',
	name: 'a/Copper.Valley.S01E02.mkv',
	size: 10,
	kind: 'video',
	guess: { title_id: 7, title_name: 'Copper Valley', title_kind: 'show', year: 2025, version_id: 3, season: 1, episode: 2, confidence: 1 },
	...over
});

const start = { value: lang.value, words: lang.words };
afterEach(() => {
	lang.value = start.value;
	lang.words = start.words;
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

	const song = (over: Partial<ByHandFile> = {}): ByHandFile => ({
		path: '/in/Brad Paisley/Part II (2001)/03.flac',
		name: '03.flac',
		size: 10,
		kind: 'audio',
		guess: { title_id: 4, title_name: 'Brad Paisley', title_kind: 'artist', version_id: 5, album_id: 6, album_name: 'Part II', album_year: 2001, disc: 1, track: 3, track_name: "I'm Gonna Miss Her", slot_id: 33, confidence: 1 },
		...over
	});

	it('matches music to an artist, album and track', () => {
		const f = song();
		const p = startPick(f);
		expect(pickLine(p)).toBe("Brad Paisley · Part II (2001) · 03 I'm Gonna Miss Her");
		expect(blocker(f, p)).toBe('');
		expect(startTicked(f, p)).toBe(true);
		expect(itemsFor([f], { [f.path]: p }, { [f.path]: true })).toEqual([{ path: f.path, title_id: 4, slot_id: 33 }]);
		const half = song({ guess: { title_id: 4, title_name: 'Brad Paisley', title_kind: 'artist', confidence: 0, reason: 'no_album', detail: 'Mud on the Tires' } });
		expect(pickLine(startPick(half))).toBe('Brad Paisley · pick the album');
		expect(blocker(half, startPick(half))).toBe('Brad Paisley has no album called Mud on the Tires in the Library. Pick the album.');
		expect(startTicked(half, startPick(half))).toBe(false);
		expect(itemsFor([half], { [half.path]: startPick(half) }, { [half.path]: true })).toEqual([]);
	});

	it('says why a music file has no match', () => {
		const none = (reason: string, detail = '') => blocker(song({ guess: { confidence: 0, reason, detail } }), null);
		expect(none('no_artist', 'Nobody')).toBe('No artist called Nobody is in the Library. Add the artist first, or pick one.');
		expect(none('no_artist')).toBe('Pick the artist, album and track.');
		expect(none('no_facts')).toBe('The file has no tags and its folders name no artist. Pick the artist, album and track.');
		const track = song({ guess: { ...song().guess, slot_id: undefined, track: undefined, track_name: undefined, reason: 'no_track' } });
		expect(blocker(track, startPick(track))).toBe('No track on Part II matches this file. Pick the track.');
		expect(pickLine(startPick(track))).toBe('Brad Paisley · Part II (2001) · pick the track');
		const filled = song({ guess: { ...song().guess, has_file: true } });
		expect(blocker(filled, startPick(filled))).toMatch(/already has a file/);
		expect(itemsFor([filled], { [filled.path]: { ...startPick(filled)!, replace: true } }, { [filled.path]: true })).toEqual([{ path: filled.path, title_id: 4, slot_id: 33, replace: true }]);
	});
	it('ticks only the best of two files guessed onto the same track', () => {
		const mp3 = song({ path: '/in/a/03.mp3', size: 5, guess: { ...song().guess, confidence: 0.8 } });
		const flac = song({ path: '/in/a/03.flac', size: 30, guess: { ...song().guess, confidence: 0.8 } });
		const other = song({ path: '/in/a/04.flac', guess: { ...song().guess, slot_id: 34 } });
		const files = [mp3, flac, other];
		const picks = Object.fromEntries(files.map((f) => [f.path, startPick(f)]));
		const ticked = Object.fromEntries(files.map((f) => [f.path, startTicked(f, picks[f.path])]));
		const held = oneTickPerTrack(files, picks, ticked);
		expect(ticked).toEqual({ [mp3.path]: false, [flac.path]: true, [other.path]: true });
		expect(held).toEqual({ [mp3.path]: true });
		const sure = song({ path: '/in/b/03.mp3', size: 1, guess: { ...song().guess, confidence: 1 } });
		const t2 = { [flac.path]: true, [sure.path]: true };
		expect(oneTickPerTrack([flac, sure], { [flac.path]: picks[flac.path], [sure.path]: startPick(sure) }, t2)).toEqual({ [flac.path]: true });
		expect(t2).toEqual({ [flac.path]: false, [sure.path]: true });
	});

	it('says why a file failed in the picked language', () => {
		lang.value = 'fr';
		lang.words = { ...(en as Record<string, string>), 'server.importer.the-file-could-not-be-placed-2': 'le fichier ne peut pas être placé' };
		expect(resultLine({ path: 'x', outcome: 'failed', error: 'the file could not be placed' })).toContain('le fichier ne peut pas être placé');
		lang.words = { ...(en as Record<string, string>), 'server.importer.hardlink-not-possible': 'lien physique impossible : {why}' };
		expect(resultLine({ path: 'x', outcome: 'placed', placed_by: 'copy', error: 'hardlink not possible: link a b: x' })).toContain('lien physique impossible : link a b: x');
	});
});
