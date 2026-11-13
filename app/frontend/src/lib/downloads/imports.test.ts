import type { ImportRecord } from './types.ts';
import { byDownload, failReason, folder, placed, statusDetail } from './imports.ts';
import { describe, expect, it } from 'vitest';

const im = (ID: number, DownloadID: number | null, State: string, more: Partial<ImportRecord> = {}): ImportRecord => ({ ID, DownloadID, State, CreatedAt: '2026-10-27T10:00:00Z', ...more });

describe('byDownload', () => {
	it('keeps the newest import of each download and skips imports without one', () => {
		const m = byDownload([im(1, 8, 'failed'), im(3, 8, 'done'), im(2, 9, 'done'), im(4, null, 'done')]);
		expect([...m.keys()].sort()).toEqual([8, 9]);
		expect(m.get(8)?.ID).toBe(3);
	});
});

describe('placed', () => {
	it('says how and where, never the whole path', () => {
		const r = im(4, 11, 'done', {
			Plan: [{ source_file: 'a.mkv', target_path: 'C:\\media\\movies\\Se7en (1995)\\Se7en (1995) 1080p.mkv' }],
			Result: [{ source_file: 'a.mkv', outcome: 'placed', placed_by: 'hardlink' }]
		});
		expect(placed(r)).toEqual({ how: 'Added to the Library (no extra space used)', where: 'Se7en (1995)' });
	});
	it('counts the files of a partly done import, leaving out skipped ones', () => {
		const r = im(5, 12, 'partly_done', {
			Result: [
				{ source_file: 'a', outcome: 'placed', placed_by: 'copy' },
				{ source_file: 'b', outcome: 'failed', error: 'disk full' },
				{ source_file: 'c', outcome: 'skipped', placed_by: 'none' }
			]
		});
		expect(placed(r)?.how).toBe('1 of 2 files added to the Library');
	});
	it('is null when nothing was placed', () => {
		expect(placed(im(1, 8, 'failed', { Result: [{ source_file: 'a', outcome: 'skipped' }] }))).toBeNull();
		expect(placed(undefined)).toBeNull();
	});
	it('folder takes the last folder of a path', () => {
		expect(folder('/data/shows/Severance/Season 01/x.mkv')).toBe('Season 01');
		expect(folder('x.mkv')).toBe('');
	});
});

describe('failReason and statusDetail', () => {
	it('says why an import failed', () => {
		expect(failReason(im(1, 8, 'failed', { Result: [{ source_file: 'a', outcome: 'skipped' }] }))).toBe('no file could be placed');
		expect(failReason(im(1, 8, 'failed', { Result: [{ source_file: 'a', outcome: 'failed', error: 'target exists' }] }))).toBe('target exists');
		expect(failReason(im(1, 8, 'done'))).toBeNull();
	});
	it('says what an import in progress is doing, the same for a torrent and a Usenet download', () => {
		expect(statusDetail({ phase: 'downloading', state: 'downloading', progress: 1 }, undefined)).toBe('unpacking');
		expect(statusDetail({ phase: 'waiting_to_import', state: 'finished', progress: 1 }, undefined)).toBe('up next');
		expect(statusDetail({ phase: 'importing', state: 'finished', progress: 1 }, undefined)).toBe('placing files');
		expect(statusDetail({ phase: 'downloading', state: 'downloading', progress: 0.5 }, undefined)).toBe('');
	});
	it('puts a failed import\'s reason on a row in Problems, and nothing on a done row', () => {
		const failed = im(1, 8, 'failed', { Result: [{ source_file: 'a', outcome: 'skipped' }] });
		expect(statusDetail({ phase: 'needs_a_decision', state: 'finished', progress: 1 }, failed)).toBe('import failed: no file could be placed');
		expect(statusDetail({ phase: 'done', state: 'finished', progress: 1 }, im(2, 9, 'done'))).toBe('');
		expect(statusDetail({ phase: 'vanished', state: 'vanished', progress: 0 }, undefined)).toBe('removed outside HolyArrs');
		expect(statusDetail({ phase: 'import_refused', state: 'finished', progress: 1 }, undefined)).toBe('the files did not match');
		expect(statusDetail({ phase: 'import_refused', state: 'finished', progress: 1, refused_why: 'the file is 50 MB, too small to be the full video; likely a fake' }, undefined)).toBe('the file is 50 MB, too small to be the full video; likely a fake');
		expect(statusDetail({ phase: 'done', state: 'seeding', progress: 1, ratio: 1.5 }, undefined)).toBe('ratio 1.50');
	});
});
