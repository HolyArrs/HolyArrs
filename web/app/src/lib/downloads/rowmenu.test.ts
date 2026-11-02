import { describe, expect, it } from 'vitest';
import { rowItems } from './rowmenu.ts';

const tor = { can_act: true, protocol: 'torrent', downloader_kind: 'qbittorrent', downloader_id: 1, hash: 'h', phase: 'downloading', state: 'downloading' };

describe('row actions menu', () => {
	it('gives a live torrent pause, queue moves and remove', () => {
		expect(rowItems(tor)).toEqual(['pause', 'up', 'down', 'top', 'bottom', 'remove']);
	});
	it('offers Resume on a paused row', () => {
		expect(rowItems({ ...tor, state: 'paused' })[0]).toBe('resume');
	});
	it('adds Open in Library for matched rows only', () => {
		expect(rowItems({ ...tor, title_id: 4 })).toContain('library');
		expect(rowItems({ ...tor, title_id: 4, outside: true })).not.toContain('library');
		expect(rowItems({ ...tor, title_id: 4, outside: true, linked: true })).toContain('library');
	});
	it('gives a failed Usenet job Try again but no pause or queue moves', () => {
		const job = { can_act: true, protocol: 'usenet', downloader_kind: 'sabnzbd', downloader_id: 2, hash: 'n', client_item: true, phase: 'failed' };
		expect(rowItems(job)).toEqual(['retry', 'remove']);
	});
	it('gives a live SABnzbd job pause and remove only', () => {
		expect(rowItems({ can_act: true, protocol: 'usenet', downloader_kind: 'sabnzbd', downloader_id: 2, hash: 'n', phase: 'downloading' })).toEqual(['pause', 'remove']);
	});
	it('keeps Transmission rows to remove', () => {
		expect(rowItems({ ...tor, downloader_kind: 'transmission' })).toEqual(['remove']);
	});
	it('never offers a client change on a read-only row', () => {
		expect(rowItems({ ...tor, can_act: false, title_id: 4 })).toEqual(['library']);
		expect(rowItems({ ...tor, can_act: false })).toEqual([]);
	});
});
