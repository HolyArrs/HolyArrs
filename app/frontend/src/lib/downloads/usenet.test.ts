import { controllable, pausable, repairSteps, retryable, serverRows, type UsenetView } from './usenet.ts';
import { describe, expect, it } from 'vitest';

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
	const base = { client_item: true, outside: true, hash: 'SABnzbd_nzo_1', downloader_id: 2, protocol: 'usenet', phase: 'failed' } as const;
	it('a failed outside Usenet job', () => expect(retryable(base)).toBe(true));
	it('not torrents, not running jobs, not HolyArrs downloads', () => {
		expect(retryable({ ...base, protocol: 'torrent' })).toBe(false);
		expect(retryable({ ...base, phase: 'downloading' })).toBe(false);
		expect(retryable({ ...base, client_item: false })).toBe(false);
		// A job HolyArrs sent was already searched again, so the server refuses Try again on it.
		expect(retryable({ ...base, outside: false })).toBe(false);
	});
});

describe('nzbget', () => {
	const job = { client_item: true, outside: true, hash: 'nzbid-7', downloader_id: 3, protocol: 'usenet', phase: 'failed', downloader_kind: 'nzbget' } as const;
	it('takes Try again on a failed job and pause on a live one', () => {
		expect(retryable(job)).toBe(true);
		expect(pausable({ protocol: 'usenet', phase: 'downloading', downloader_kind: 'nzbget' })).toBe(true);
		expect(retryable({ ...job, downloader_kind: 'deluge' as never })).toBe(false);
	});
	it('shows the move step in the Repair tab', () => {
		expect(repairSteps({ stages: [{ stage: 'move', lines: ['Move to the finished folder: failed'] }], job_servers: [] })).toEqual([{ title: 'Move', lines: ['Move to the finished folder: failed'] }]);
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

describe('controllable', () => {
	it('offers row controls only for engines that take them', () => {
		expect(controllable('qbittorrent')).toBe(true);
		expect(controllable('sabnzbd')).toBe(true);
		expect(controllable(undefined)).toBe(true);
		for (const k of ['transmission', 'deluge', 'nzbget', 'rtorrent']) expect(controllable(k)).toBe(false);
		expect(pausable({ protocol: 'torrent', phase: 'downloading', downloader_kind: 'deluge' })).toBe(false);
	});
	it('lets rTorrent pause and resume but take no other row control', () => {
		expect(controllable('rtorrent')).toBe(false);
		expect(pausable({ protocol: 'torrent', phase: 'downloading', downloader_kind: 'rtorrent' })).toBe(true);
		expect(pausable({ protocol: 'torrent', phase: 'done', downloader_kind: 'rtorrent' })).toBe(true);
		expect(retryable({ client_item: true, outside: true, hash: 'h', downloader_id: 3, protocol: 'torrent', phase: 'failed', downloader_kind: 'rtorrent' })).toBe(false);
	});
});
