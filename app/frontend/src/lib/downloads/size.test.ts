import { protocolOf, size, freeSize } from './size.ts';
import { describe, expect, it } from 'vitest';

describe('size', () => {
	it('uses 1000-based steps', () => {
		expect(size(0)).toBe('0 B');
		expect(size(999_000)).toBe('999 KB');
		expect(size(1_000_000)).toBe('1 MB');
		expect(size(553_000_000)).toBe('553 MB');
		expect(size(1_000_000_000)).toBe('1 GB');
		expect(size(1_690_000_000)).toBe('1.7 GB');
		expect(size(3_000_000_000)).toBe('3 GB');
		expect(size(18_400_000_000)).toBe('18 GB');
		expect(size(1_000_000_000_000)).toBe('1 TB');
		expect(size(999_960_000)).toBe('1 GB');
	});
	it('names the protocol', () => {
		expect(protocolOf({ protocol: 'usenet' })).toBe('usenet');
		expect(protocolOf({ downloader_kind: 'qbittorrent' })).toBe('torrent');
		expect(protocolOf({ downloader_kind: 'rtorrent' })).toBe('torrent');
		expect(protocolOf({})).toBe('');
	});
});

import { categoryWords } from './size.ts';
describe('categoryWords', () => {
	it('reads client categories in plain words', () => {
		expect(categoryWords('tv_shows')).toBe('TV show');
		expect(categoryWords('sonarr')).toBe('TV show');
		expect(categoryWords('movies')).toBe('Movie');
		expect(categoryWords('radarr')).toBe('Movie');
		expect(categoryWords('linux_isos')).toBe('Linux isos');
		expect(categoryWords('')).toBe('');
	});
});

describe('freeSize', () => {
	it('shows disk space the way the Downloaders do: 1024 steps, one decimal', () => {
		// qBittorrent's 14.152 TiB, SABnzbd's "14.2 T"
		expect(freeSize(14.152 * 1024 ** 4)).toBe('14.2 TB');
		expect(freeSize(10 * 1024 ** 4)).toBe('10.0 TB');
		expect(freeSize(512.34 * 1024 ** 3)).toBe('512.3 GB');
		expect(freeSize(700 * 1024 ** 2)).toBe('700 MB');
		expect(freeSize(0)).toBe('0 B');
		expect(freeSize(1023.96 * 1024 ** 3)).toBe('1.0 TB');
		expect(freeSize(1023.6 * 1024 ** 2)).toBe('1.0 GB');
	});
	it('leaves download sizes as they are', () => {
		expect(size(14.152 * 1024 ** 4)).toBe('16 TB');
	});
});
