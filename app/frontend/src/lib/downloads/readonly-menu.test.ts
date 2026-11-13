import { paneExtras, rowItems } from './rowmenu.ts';
import { describe, expect, it } from 'vitest';

const torrent = { can_act: true, protocol: 'torrent', downloader_kind: 'qbittorrent', downloader_id: 1, hash: 'abc', phase: 'downloading', state: 'downloading' };
const job = { can_act: true, protocol: 'usenet', downloader_kind: 'sabnzbd', downloader_id: 2, hash: 'SABnzbd_nzo_1', phase: 'downloading', state: 'downloading' };

describe("the pane's ⋯ menu", () => {
	it('adds checks, force start, the sheets and Mark failed to a torrent', () => {
		expect(paneExtras(torrent, { steer: true, fail: true })).toEqual(['recheck', 'reannounce', 'force', 'steer', 'fail']);
		expect(paneExtras(torrent, { steer: false, fail: false })).toEqual(['recheck', 'reannounce']);
	});
	it('offers Transmission, Deluge, rTorrent and aria2 none of the qBittorrent-only calls', () => {
		expect(paneExtras({ ...torrent, downloader_kind: 'transmission' }, { steer: true, fail: true })).toEqual(['fail']);
		expect(paneExtras({ ...torrent, downloader_kind: 'deluge' }, { steer: true, fail: false })).toEqual([]);
		expect(paneExtras({ ...torrent, downloader_kind: 'rtorrent' }, { steer: true, fail: true })).toEqual(['fail']);
		expect(paneExtras({ ...torrent, downloader_kind: 'rtorrent', can_act: false }, { steer: true, fail: true })).toEqual([]);
		expect(rowItems({ ...torrent, downloader_kind: 'rtorrent', can_act: false })).toEqual([]);
		expect(paneExtras({ ...torrent, downloader_kind: 'aria2' }, { steer: true, fail: true })).toEqual(['fail']);
		expect(paneExtras({ ...torrent, downloader_kind: 'aria2', can_act: false }, { steer: true, fail: true })).toEqual([]);
		expect(rowItems({ ...torrent, downloader_kind: 'aria2', can_act: false })).toEqual([]);
	});
	it('gives a Usenet job only Mark failed beyond its row list, which has its queue moves and priority', () => {
		expect(paneExtras(job, { steer: true, fail: true })).toEqual(['fail']);
		expect(rowItems(job)).toEqual(['pause', 'up', 'down', 'top', 'bottom', 'priority', 'remove']);
	});
	it('offers a read-only row nothing that changes the client', () => {
		expect(paneExtras({ ...torrent, can_act: false }, { steer: true, fail: true })).toEqual([]);
		expect(paneExtras({ ...job, can_act: false }, { steer: true, fail: true })).toEqual([]);
		expect(rowItems({ ...torrent, can_act: false })).toEqual([]);
	});
	it("keeps only HolyArrs' own records on a row whose Downloader is locked", () => {
		const own = { ...torrent, client_locked: true, title_id: 7 };
		expect(rowItems(own)).toEqual(['library', 'remove']);
		expect(paneExtras(own, { steer: true, fail: true })).toEqual(['fail']);
		expect(rowItems({ ...job, client_locked: true })).toEqual(['remove']);
	});
	it('never leaves a row stuck before its import without a way on', () => {
		const finished = { ...torrent, phase: 'waiting_to_import', state: 'finished', title_id: 7 };
		// The Downloader was removed: Remove (records only) and Import again.
		expect(rowItems({ ...finished, can_act: true, client_locked: true, can_import: true })).toEqual(['import', 'library', 'remove']);
		// Still on a live Downloader: Import again beside Open in Library.
		expect(rowItems({ ...finished, can_act: false, can_import: true })).toEqual(['import', 'library']);
	});
});
