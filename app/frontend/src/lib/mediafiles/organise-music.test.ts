import { describe, expect, it } from 'vitest';
import {
	albumFolderLine,
	albumSteps,
	cutFilesLine,
	choices,
	deleteBody,
	deleteSummary,
	groupPlan,
	musicExampleRows,
	musicFolderLine,
	renamePair,
	trackFileLine,
	wholeScope,
	type AdoptAlbum,
	type AdoptCandidate,
	type RemovePlan,
	type RenameStep
} from './organise.ts';

describe('music library import', () => {
	const album: AdoptAlbum = {
		folder: 'Old Days (2010)',
		parsed_name: 'Old Days',
		parsed_year: 2010,
		match: { id: '10000000-0000-0000-0000-000000000001', tmdb_id: '', name: 'Old Days', year: 2010, score: 1 },
		pressing: 'Old Days (2011, GB, CD, 4 tracks)',
		tracks: 4,
		files: [
			{ path: 'Lab Band/Old Days (2010)/CD1/01 - Song 1.flac', size: 1, track: 'disc 1 track 01 Song 1' },
			{ path: 'Lab Band/Old Days (2010)/x.flac', size: 1, why: 'the file name gives no track number or track name; left alone' }
		],
		matched: 1,
		unknown: 1,
		status: 'matched',
		why: ''
	};
	const artist: AdoptCandidate = {
		folder: 'Lab Band',
		parsed_name: 'Lab Band',
		videos: [],
		match: { id: 'mbid:a0000000-0000-0000-0000-000000000001', tmdb_id: '', name: 'Lab Band', year: 0, score: 1 },
		others: [],
		status: 'matched',
		why: '',
		albums: [album, { ...album, folder: 'Demos', match: undefined, pressing: undefined, status: 'unmatched', matched: 0, unknown: 1 }],
		tracks: 1,
		unknown: 2
	};

	it('sums up an artist folder, its albums and files', () => {
		expect(musicFolderLine(artist)).toBe('1 track in 1 album; 2 files left alone');
		expect(albumFolderLine(album)).toBe('Old Days (2010) · 100% · 1 of 2 files hold a track of Old Days (2011, GB, CD, 4 tracks)');
		expect(albumFolderLine(artist.albums![1])).toBe('No album fits; 2 files left alone');
		expect(trackFileLine(album.files[0])).toBe('disc 1 track 01 Song 1');
		const big: AdoptAlbum = { ...album, cut: true, matched: 50, unknown: 10 };
		expect(albumFolderLine(big)).toBe('Old Days (2010) · 100% · 50 of 60 files hold a track of Old Days (2011, GB, CD, 4 tracks)');
		expect(cutFilesLine(big)).toBe('and 58 more files, not listed');
		expect(cutFilesLine(album)).toBe('');
		expect(trackFileLine(album.files[1])).toMatch(/^Left alone: the file name gives no track/);
	});

	it('adopts artist folders by their MusicBrainz id and words groups for artists', () => {
		expect(choices({ 'Lab Band': 'mbid:x', Other: '' }, 'music')).toEqual([{ folder: 'Lab Band', id: 'mbid:x' }]);
		const g = groupPlan([artist, { ...artist, folder: 'Empty', status: 'no_video' }], 'artist');
		expect(g[0].about).toBe('HolyArrs is sure which artist these are.');
		expect(g[1].label).toBe('No audio file');
		expect(groupPlan([artist])[0].about).toBe('HolyArrs is sure which movie these are.');
	});
});

describe('music rename and delete', () => {
	const steps: RenameStep[] = [
		{ file_id: 1, title_id: 9, title_name: 'Lab Band', origin: 'imported', from: 'Lab Band/old/a.flac', to: 'Lab Band/Old Days (2010)/01 - Song 1.flac', album_id: 3 },
		{ file_id: 2, title_id: 9, title_name: 'Lab Band', origin: 'imported', from: 'Lab Band/Middle (2018)/b.mp3', to: 'Lab Band/Middle (2018)/01 - Song m1.mp3', album_id: 4 }
	];

	it('keeps one album of an artist rename', () => {
		expect(albumSteps(steps, 3).map((s) => s.file_id)).toEqual([1]);
		expect(albumSteps(steps, undefined)).toHaveLength(2);
	});

	it('shows whole paths when the album folder changes', () => {
		expect(renamePair(steps[0])).toEqual(['Lab Band/old/a.flac', 'Lab Band/Old Days (2010)/01 - Song 1.flac']);
		expect(renamePair(steps[1])).toEqual(['b.mp3', '01 - Song m1.mp3']);
		const win = { ...steps[0], from: 'Lab Band\\old\\a.flac', to: 'Lab Band\\Old Days (2010)\\a.flac' };
		expect(renamePair(win)).toEqual([win.from, win.to]);
	});

	it('asks for an album, tracks or the whole artist', () => {
		expect(deleteBody({ albumId: 3 }, true, true, false)).toEqual({ delete_files: true, apply: false, album_id: 3 });
		expect(deleteBody({ trackIds: [5, 6] }, false, false, true)).toEqual({ delete_files: false, apply: true, track_ids: [5, 6] });
		expect(deleteBody({}, true, true, true)).toEqual({ delete_files: true, apply: true, never_add: true });
		expect(deleteBody({ season: 1 }, false, true, false)).toEqual({ delete_files: false, apply: false, season: 1 });
		expect(wholeScope({})).toBe(true);
		expect(wholeScope({ albumId: 1 })).toBe(false);
		expect(wholeScope({ trackIds: [1] })).toBe(false);
	});

	it('says what an album delete does', () => {
		const p: RemovePlan = {
			title_id: 9,
			name: 'Lab Band',
			scope: 'album',
			album: 'Old Days',
			tracks: ['Old Days: track 01 Song 1', 'Old Days: track 02 Song 2'],
			choice: { delete_files: true, never_add: false },
			applied: false,
			held: [{ file_id: 1, origin: 'adopted', path: 'a', why: '' }],
			kept: [],
			holding_days: 7,
			active_downloads: 0
		};
		const s = deleteSummary(p, 'artist');
		expect(s[0]).toBe('Old Days: 2 tracks stop being kept up to date');
		expect(s[1]).toMatch(/^1 file to the holding folder/);
		expect(deleteSummary({ ...p, scope: 'title' }, 'artist')[0]).toMatch(/^Leaves your Library/);
	});

	it('lists the music examples in order', () => {
		expect(musicExampleRows({ various: 'V', track: 'T', multi_disc: 'M' }).map(([l]) => l)).toEqual([
			'A track',
			'A track of a two-disc album',
			'A track of a various-artists album'
		]);
		expect(musicExampleRows(undefined)).toEqual([]);
	});
});
