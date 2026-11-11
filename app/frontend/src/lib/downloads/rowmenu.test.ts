import { rowItems } from './rowmenu.ts';
import { describe, expect, it } from 'vitest';

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
		const job = { can_act: true, protocol: 'usenet', downloader_kind: 'sabnzbd', downloader_id: 2, hash: 'n', client_item: true, outside: true, phase: 'failed' };
		expect(rowItems(job)).toEqual(['retry', 'remove']);
	});
	it('gives a live SABnzbd or NZBGet job pause, queue moves, priority and remove', () => {
		for (const kind of ['sabnzbd', 'nzbget'])
			expect(rowItems({ can_act: true, protocol: 'usenet', downloader_kind: kind, downloader_id: 2, hash: 'n', phase: 'downloading' })).toEqual(['pause', 'up', 'down', 'top', 'bottom', 'priority', 'remove']);
		expect(rowItems({ can_act: true, protocol: 'usenet', downloader_kind: 'nzbget', downloader_id: 2, hash: 'nzbid-4', phase: 'paused', state: 'paused' })[0]).toBe('resume');
	});
	it('keeps queue moves and priority off finished Usenet jobs', () => {
		expect(rowItems({ can_act: true, protocol: 'usenet', downloader_kind: 'nzbget', downloader_id: 2, hash: 'n', phase: 'done' })).toEqual(['remove']);
		expect(rowItems({ can_act: false, protocol: 'usenet', downloader_kind: 'sabnzbd', downloader_id: 2, hash: 'n', phase: 'downloading' })).toEqual([]);
	});
	it('keeps Transmission rows to remove', () => {
		expect(rowItems({ ...tor, downloader_kind: 'transmission' })).toEqual(['remove']);
	});
	it('gives rTorrent rows pause or resume and remove only', () => {
		expect(rowItems({ ...tor, downloader_kind: 'rtorrent' })).toEqual(['pause', 'remove']);
		expect(rowItems({ ...tor, downloader_kind: 'rtorrent', state: 'paused' })).toEqual(['resume', 'remove']);
		expect(rowItems({ ...tor, downloader_kind: 'rtorrent', can_act: false })).toEqual([]);
	});
	it('never offers a client change on a read-only row', () => {
		expect(rowItems({ ...tor, can_act: false, title_id: 4 })).toEqual(['library']);
		expect(rowItems({ ...tor, can_act: false })).toEqual([]);
	});
});
