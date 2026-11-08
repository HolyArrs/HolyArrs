import { kindOf, rowPath } from './downloaders.ts';
import { counts, stageOf, statusWord, TABS } from './status-tabs.ts';
import { historyStatus } from './sections.ts';
import { describe, expect, it } from 'vitest';
import { type DownloadRow } from '../core/types.ts';

const item = (over: Partial<DownloadRow>): DownloadRow => ({
	id: -1_000_000_000, phase: 'done', state: 'seeding', label: 'Done', text: '', progress: 1, speed: 0, active: false, name: 'Linux.ISO', title: '',
	version: '', downloader: 'qBittorrent', protocol: 'torrent', size: 5000, updated_at: '2026-10-30T00:00:00Z', can_act: true,
	client_item: true, outside: true, downloader_id: 1, hash: '1111111111111111111111111111111111111111', category: 'linux', ...over
});

describe('client items', () => {
	it('are addressed by Downloader and hash, HolyArrs rows by id', () => {
		expect(rowPath(item({}))).toBe('/downloaders/1/items/1111111111111111111111111111111111111111');
		expect(rowPath({ id: 7 })).toBe('/downloads/7');
	});
	it('have no library type unless HolyArrs knows the hash', () => {
		expect(kindOf(item({}))).toBe('');
		expect(kindOf(item({ kind: 'show' }))).toBe('show');
		expect(kindOf({ kind: undefined })).toBe('movie');
	});
	it('count a seeding torrent under Done with Seeding as its word', () => {
		const r = item({});
		expect(stageOf(r)).toBe('seeding');
		expect(statusWord(r)).toBe('Seeding');
		expect(counts([r], TABS).find((c) => c.key === 'done')?.count).toBe(1);
		expect(historyStatus(r, undefined).text).toBe('Seeding');
		expect(statusWord(item({ state: 'paused' }))).toBe('Done');
		expect(statusWord(item({ phase: 'stalled', state: 'stalled', active: true }))).toBe('Stalled');
	});
	it('show the Downloader reason on a failed item', () => {
		const failed = item({ phase: 'failed', state: 'failed', label: 'Failed', progress: 0, protocol: 'usenet', downloader: 'SABnzbd' });
		expect(historyStatus({ ...failed, text: 'Download failed - Not on your server(s)' }, undefined)).toEqual({ text: 'Failed: Download failed - Not on your server(s)', bad: true });
		expect(historyStatus(failed, undefined)).toEqual({ text: 'Failed', bad: true });
	});
});
