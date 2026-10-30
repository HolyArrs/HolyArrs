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
