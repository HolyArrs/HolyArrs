import { addWords, isTorrentFile, splitLinks } from './manage.ts';
import { describe, expect, it } from 'vitest';

describe('splitLinks', () => {
	it('keeps magnets, http links and hashes, one per line', () => {
		const h = 'a'.repeat(40);
		expect(splitLinks(`magnet:?xt=urn:btih:${h}\n\n  https://x.test/a.torrent \r\n${h}\nftp://nope`)).toEqual({
			links: [`magnet:?xt=urn:btih:${h}`, 'https://x.test/a.torrent', h],
			bad: ['ftp://nope']
		});
	});
});

describe('addWords', () => {
	it('names every part of the outcome', () => {
		expect(addWords({ added: 2, failed: 0, pending: 1, duplicates: 1 })).toEqual({ text: '2 torrents added', body: '1 still being fetched · 1 already there', tone: 'ok' });
		expect(addWords({ added: 1, failed: 1, pending: 0, duplicates: 0 }).tone).toBe('info');
		expect(addWords({ added: 0, failed: 2, pending: 0, duplicates: 0 })).toEqual({ text: '2 failed', tone: 'fail' });
		expect(addWords({ added: 0, failed: 0, pending: 0, duplicates: 3 })).toEqual({ text: '3 already there', tone: 'info' });
		expect(addWords({ added: 0, failed: 1, pending: 0, duplicates: 0, bad: ['x.txt'] })).toMatchObject({ text: '1 failed', body: 'could not add: x.txt' });
		expect(addWords({ added: 1, failed: 0, pending: 0, duplicates: 0, dropped: 2 })).toMatchObject({ text: '1 torrent added', body: '2 local addresses left out of the magnet links' });
	});
});

it('isTorrentFile', () => {
	expect(isTorrentFile({ name: 'A.TORRENT' })).toBe(true);
	expect(isTorrentFile({ name: 'a.txt', type: 'application/x-bittorrent' })).toBe(true);
	expect(isTorrentFile({ name: 'a.txt' })).toBe(false);
});

import { byClient, pickRange } from './manage.ts';
describe('selection', () => {
	it('pickRange works both ways', () => {
		expect(pickRange([1, 2, 3, 4], 2, 4)).toEqual([2, 3, 4]);
		expect(pickRange([1, 2, 3, 4], 4, 1)).toEqual([1, 2, 3, 4]);
		expect(pickRange([1, 2], 9, 2)).toEqual([2]);
		expect(pickRange([1, 2], 1, 9)).toEqual([]);
	});
	it('byClient groups torrent rows with a hash', () => {
		const m = byClient([
			{ downloader_id: 1, hash: 'a', protocol: 'torrent' },
			{ downloader_id: 2, hash: 'b' },
			{ downloader_id: 1, hash: 'c', protocol: 'torrent' },
			{ downloader_id: 3, protocol: 'usenet' },
			{ downloader_id: 4, hash: 'd', protocol: 'usenet' }
		]);
		expect([...m.keys()]).toEqual([1, 2]);
		expect(m.get(1)?.map((r) => r.hash)).toEqual(['a', 'c']);
	});
});

import { checkLimits, limitsWords, modeOf, valueOf } from './manage.ts';
describe('seeding rules', () => {
	it('reads limits in plain words', () => {
		expect(limitsWords(undefined)).toBe('–');
		expect(limitsWords({ ratio: -2, seeding_minutes: -2, inactive_minutes: -2, action: '' })).toBe("Downloader's rules");
		expect(limitsWords({ ratio: 2, seeding_minutes: 1440, inactive_minutes: 90, action: 'stop' })).toBe('Ratio 2 · 1 day · 90 minutes idle, then stop');
		expect(limitsWords({ ratio: -1, seeding_minutes: 120, inactive_minutes: -2, action: 'remove' })).toBe('No ratio limit · 2 hours, then remove');
	});
	it('maps modes and checks values', () => {
		expect(modeOf(-2)).toBe('global');
		expect(modeOf(-1)).toBe('none');
		expect(modeOf(3)).toBe('set');
		expect(valueOf('none', 5)).toBe(-1);
		expect(valueOf('set', 5)).toBe(5);
		expect(checkLimits({ ratio: 10000, seeding_minutes: -2, inactive_minutes: -2, action: '' })).toMatch(/Ratio/);
		expect(checkLimits({ ratio: 1, seeding_minutes: 1.5, inactive_minutes: -2, action: '' })).toMatch(/Minutes/);
		expect(checkLimits({ ratio: 1, seeding_minutes: 60, inactive_minutes: -1, action: '' })).toBe('');
	});
});

import { byTracker, splitTrackers, trackerOptions } from './manage.ts';
describe('tracker filter', () => {
	const rows = [{ tracker: 'b.example' }, { tracker: 'a.example', tracker_problem: 'warning' as const }, { tracker_problem: 'announce' as const }, { tracker: 'b.example' }];
	it('lists problems first, then hosts, with counts', () => {
		expect(trackerOptions(rows)).toEqual([
			{ key: 'p:warning', label: 'Tracker warning', count: 1 },
			{ key: 'p:announce', label: 'No tracker reply', count: 1 },
			{ key: 'host:a.example', label: 'a.example', count: 1 },
			{ key: 'host:b.example', label: 'b.example', count: 2 }
		]);
	});
	it('filters by host or problem', () => {
		expect(rows.filter((r) => byTracker(r, 'host:b.example'))).toHaveLength(2);
		expect(rows.filter((r) => byTracker(r, 'p:announce'))).toHaveLength(1);
		expect(rows.filter((r) => byTracker(r, ''))).toHaveLength(4);
	});
	it('reads typed trackers', () => {
		expect(splitTrackers('udp://a.example:80\n\nhttps://b.example/x\nftp://c\nudp://a|b')).toEqual({ urls: ['udp://a.example:80', 'https://b.example/x'], bad: ['ftp://c', 'udp://a|b'] });
	});
});

import { bytesOf, kbOf } from './manage.ts';
describe('speed boxes', () => {
	it('read and write KB/s', () => {
		expect(kbOf(2000)).toBe(2);
		expect(kbOf(undefined)).toBe(0);
		expect(bytesOf('1.5')).toBe(1500);
		expect(bytesOf('')).toBe(0);
		expect(bytesOf(-3)).toBe(0);
	});
});

import { filterFiles, indexesUnder } from './manage.ts';
describe('files tab', () => {
	const files = [
		{ index: 0, name: 'Show/S01/e01.mkv' },
		{ index: 1, name: 'Show/S01/e02.mkv' },
		{ index: 2, name: 'Show/Sample/sample.mkv' },
		{ index: 3, name: 'Show/info.nfo' }
	];
	it('filters by every typed word', () => {
		expect(filterFiles(files, 'mkv s01').map((f) => f.index)).toEqual([0, 1]);
		expect(filterFiles(files, '  ')).toHaveLength(4);
		expect(filterFiles(files, 'SAMPLE').map((f) => f.index)).toEqual([2]);
	});
	it('collects indexes under folders and files once', () => {
		expect(indexesUnder(files, ['Show/S01', 'Show/S01/e02.mkv', 'Show/info.nfo'])).toEqual([0, 1, 3]);
		expect(indexesUnder(files, ['Show/S0'])).toEqual([]);
	});
});

import { byTracker as byTr, problemWords } from './manage.ts';
describe('tracker problems', () => {
	it('p:any passes every row with a tracker problem; the chip counts them', () => {
		expect(byTr({ tracker: 'a', tracker_problem: 'error' }, 'p:any')).toBe(true);
		expect(byTr({ tracker: 'a' }, 'p:any')).toBe(false);
		expect(problemWords(0)).toBe('');
		expect(problemWords(1)).toBe('1 tracker issue');
		expect(problemWords(3)).toBe('3 tracker issues');
	});
});

import { countLabel, toolbarTargets } from './manage.ts';
describe('the toolbar targets', () => {
	const paused = { can_act: true, state: 'paused' };
	const running = { can_act: true, state: 'downloading' };
	const locked = { can_act: false, state: 'downloading' };
	it('mean the ticked rows first, else the open row', () => {
		expect(toolbarTargets([paused, running], undefined, 'paused').rows).toEqual([paused]);
		expect(toolbarTargets([], running, 'running').rows).toEqual([running]);
		expect(toolbarTargets([paused], running, 'any').rows).toEqual([paused]);
	});
	it('say why they are off', () => {
		expect(toolbarTargets([], undefined, 'any').why).toBe('select a download');
		expect(toolbarTargets([locked], undefined, 'any').why).toBe('this Downloader is read-only');
		expect(toolbarTargets([], locked, 'running').why).toBe('this Downloader is read-only');
		expect(toolbarTargets([running], undefined, 'paused').why).toBe('select a paused download');
		expect(toolbarTargets([paused], undefined, 'running').why).toBe('select a running download');
		expect(toolbarTargets([running], undefined, 'running', () => false).why).toBe('not for this Downloader');
	});
	it('carry the count in the label only past one', () => {
		expect(countLabel('Pause', 1)).toBe('Pause');
		expect(countLabel('Pause', 3)).toBe('Pause 3');
	});
});

import { toolbarTargets as tt } from './manage.ts';
describe('a locked Downloader', () => {
	it('says read-only for Start and Pause but still lets Remove act', () => {
		const r = { protocol: 'torrent', state: 'paused', can_act: true, client_locked: true };
		expect(tt([r], undefined, 'paused', (x) => !x.client_locked)).toEqual({ rows: [], why: 'this Downloader is read-only' });
		expect(tt([r], undefined, 'any').rows).toHaveLength(1);
	});
});
