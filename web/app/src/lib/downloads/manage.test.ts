import { describe, expect, it } from 'vitest';
import { addWords, isTorrentFile, splitLinks } from './manage.ts';

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
		expect(addWords({ added: 2, failed: 0, pending: 1, duplicates: 1 })).toEqual({ text: '2 torrents added · 1 still being fetched · 1 already there.', tone: 'ok' });
		expect(addWords({ added: 1, failed: 1, pending: 0, duplicates: 0 }).tone).toBe('info');
		expect(addWords({ added: 0, failed: 2, pending: 0, duplicates: 0 })).toEqual({ text: '2 failed.', tone: 'fail' });
		expect(addWords({ added: 0, failed: 0, pending: 0, duplicates: 3 })).toEqual({ text: '3 already there.', tone: 'info' });
		expect(addWords({ added: 0, failed: 1, pending: 0, duplicates: 0, bad: ['x.txt'] }).text).toBe('1 failed · could not add: x.txt.');
		expect(addWords({ added: 1, failed: 0, pending: 0, duplicates: 0, dropped: 2 }).text).toBe('1 torrent added · 2 local addresses left out of the magnet links.');
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
			{ key: 'p:announce', label: 'Not answering', count: 1 },
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
