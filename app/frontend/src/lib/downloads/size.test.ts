import { protocolOf, size } from './size.ts';
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
