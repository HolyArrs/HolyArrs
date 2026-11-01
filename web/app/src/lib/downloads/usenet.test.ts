import { describe, expect, it } from 'vitest';
import { pausable, repairSteps, retryable, serverRows, type UsenetView } from './usenet.ts';

const view: UsenetView = {
	stages: [
		{ stage: 'source', lines: ['https://idx.example'] },
		{ stage: 'servers', lines: ['news.example=812 MB'] },
		{ stage: 'repair', lines: ['Verified in 2 seconds'] },
		{ stage: 'unpack', lines: ['Unpacked 1 file'] },
		{ stage: 'script', lines: [] }
	],
	job_servers: [{ name: 'news.example', amount: '812 MB' }],
	servers: { total: 900, month: 1, week: 1, day: 1, servers: [{ name: 'news.example', total: 900, month: 1, week: 1, day: 1, articles_tried: 10, articles_found: 9 }, { name: 'backup', total: 5, month: 0, week: 0, day: 0, articles_tried: 0, articles_found: 0 }] }
};

describe('retryable', () => {
	const base = { client_item: true, hash: 'SABnzbd_nzo_1', downloader_id: 2, protocol: 'usenet', phase: 'failed' } as const;
	it('a failed outside Usenet job', () => expect(retryable(base)).toBe(true));
	it('not torrents, not running jobs, not HolyArrs downloads', () => {
		expect(retryable({ ...base, protocol: 'torrent' })).toBe(false);
		expect(retryable({ ...base, phase: 'downloading' })).toBe(false);
		expect(retryable({ ...base, client_item: false })).toBe(false);
	});
});

describe('repairSteps', () => {
	it('keeps the post-download stages with lines, in order', () => {
		expect(repairSteps(view)).toEqual([{ title: 'Verify and repair', lines: ['Verified in 2 seconds'] }, { title: 'Unpack', lines: ['Unpacked 1 file'] }]);
	});
	it('nothing read yet', () => expect(repairSteps(undefined)).toEqual([]));
});

describe('serverRows', () => {
	it('joins the job amounts with the totals', () => {
		expect(serverRows(view)).toEqual([
			{ name: 'news.example', job: '812 MB', total: 900, found: '90%' },
			{ name: 'backup', job: '', total: 5, found: '' }
		]);
	});
});

describe('sab audit fixes', () => {
	it('shows the download stage first so a failed job says why', () => {
		const v: UsenetView = { stages: [{ stage: 'unpack', lines: ['x'] }, { stage: 'download', lines: ['136 articles were missing'] }] } as UsenetView;
		expect(repairSteps(v)[0]).toEqual({ title: 'Download', lines: ['136 articles were missing'] });
	});
	it('offers no pause on finished or failed Usenet jobs', () => {
		expect(pausable({ protocol: 'usenet', phase: 'failed' })).toBe(false);
		expect(pausable({ protocol: 'usenet', phase: 'done' })).toBe(false);
		expect(pausable({ protocol: 'usenet', phase: 'downloading' })).toBe(true);
		expect(pausable({ protocol: 'torrent', phase: 'done' })).toBe(true);
	});
});
