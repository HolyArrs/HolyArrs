import { describe, expect, it } from 'vitest';
import type { DownloadRow } from '../downloaders.ts';
import type { ImportRecord } from '../types.ts';
import { activityOf, detailFacts, filesOf, iconOf } from './details.ts';

const row = (more: Partial<DownloadRow> = {}): DownloadRow => ({
	id: 1,
	phase: 'downloading',
	state: 'downloading',
	label: '',
	text: '',
	progress: 0.4,
	speed: 2 * 1024 * 1024,
	eta: 600,
	active: true,
	name: 'Some.Show.S10E06.1080p',
	title: 'The Great British Bake Off',
	kind: 'show',
	covers: 'S10E06',
	version: 'Main',
	downloader: 'qBittorrent',
	size: 3 * 1024 ** 3,
	updated_at: '2026-10-27T10:00:00Z',
	can_act: true,
	...more
});
const keys = (fs: { key: string }[]) => fs.map((f) => f.key);

describe('detailFacts', () => {
	it('says what it is, where it is and what happens next for a running download', () => {
		const f = detailFacts(row(), undefined);
		expect(keys(f.what)).toEqual(['covers', 'size']);
		expect(f.what[0]).toEqual({ key: 'covers', label: 'Episode', value: 'S10E06' });
		expect(keys(f.where)).toEqual(['progress', 'speed', 'eta', 'downloader']);
		expect(f.where[0].value).toBe('40%');
		expect(f.next).toEqual([]);
		expect(f.line).toMatch(/imports it/);
		expect(f.steps.map((s) => s.state)).toEqual(['now', 'next', 'next']);
	});
	it('hides empty facts and the Main Version', () => {
		const f = detailFacts(row({ kind: 'movie', covers: undefined, size: 0, downloader: '', active: false, phase: 'waiting', progress: 0, speed: 0, eta: undefined }), undefined);
		expect(f.what).toEqual([]);
		expect(keys(f.where)).toEqual(['progress']);
		expect(f.stage).toBe('queued');
	});
	it('shows a named Version and leaves progress out of a failed download', () => {
		const f = detailFacts(row({ kind: 'movie', version: '4K', phase: 'failed', active: false }), undefined);
		expect(f.what[0]).toEqual({ key: 'version', label: 'Version', value: '4K' });
		expect(keys(f.where)).toEqual(['downloader']);
		expect(f.moving).toBe(false);
		expect(f.steps[0].state).toBe('fail');
	});
	it('tells where a done download went', () => {
		const im: ImportRecord = {
			ID: 3,
			DownloadID: 1,
			State: 'done',
			CreatedAt: '2026-10-27T11:00:00Z',
			Plan: [{ source_file: '/dl/a.mkv', target_path: '/media/movies/Se7en (1995)/Se7en.mkv' }],
			Result: [{ source_file: '/dl/a.mkv', outcome: 'placed', placed_by: 'hardlink' }]
		};
		const f = detailFacts(row({ kind: 'movie', phase: 'done', active: false, progress: 1 }), im);
		expect(f.next).toEqual([
			{ key: 'import', label: 'Import', value: 'Placed by hardlink' },
			{ key: 'folder', label: 'Folder', value: 'Se7en (1995)' }
		]);
		expect(f.line).toBe('In the Library in Se7en (1995).');
		expect(f.steps.every((s) => s.state === 'done')).toBe(true);
		expect(filesOf(im)).toEqual([{ name: 'a.mkv', to: 'Se7en (1995)', outcome: 'placed' }]);
	});
	it('marks the import step for a download that needs a decision', () => {
		const f = detailFacts(row({ phase: 'needs_a_decision', active: false, progress: 1 }), undefined);
		expect(f.stage).toBe('decide');
		expect(f.steps.map((s) => s.state)).toEqual(['done', 'fail', 'next']);
	});
	it('gives a seeding torrent its ratio', () => {
		const f = detailFacts(row({ phase: 'done', state: 'seeding', active: false, progress: 1, ratio: 1.234 }), undefined);
		expect(f.where.find((x) => x.key === 'ratio')?.value).toBe('1.23');
	});
});

describe('iconOf', () => {
	it('picks one symbol per stage', () => {
		expect(iconOf(row())).toBe('download');
		expect(iconOf(row({ phase: 'paused' }))).toBe('pause');
		expect(iconOf(row({ phase: 'failed' }))).toBe('x');
		expect(iconOf(row({ phase: 'removed' }))).toBe('trash');
		expect(iconOf(row({ phase: 'done', state: 'seeding' }))).toBe('upload');
	});
});

describe('activityOf', () => {
	it('lists what happened in time order', () => {
		const a = activityOf({ started_at: '2026-10-27T09:00:00Z', finished_at: '2026-10-27T10:00:00Z', updated_at: '2026-10-27T10:00:00Z' }, { ID: 1, State: 'failed', CreatedAt: '2026-10-27T10:05:00Z' });
		expect(a.map((x) => x.key)).toEqual(['started', 'finished', 'import']);
		expect(a[2].label).toBe('Import failed');
	});
	it('falls back to the last change', () => {
		expect(activityOf({ updated_at: '2026-10-27T10:00:00Z' }, undefined).map((x) => x.key)).toEqual(['updated']);
	});
});
