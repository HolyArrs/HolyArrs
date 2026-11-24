import { describe, expect, it } from 'vitest';
import type { DownloadRow } from '../core/types.ts';
import { infoGroups, usenetGroups } from './pane-facts.ts';

const row = { progress: 0.5, size: 1000, speed: 10, up_speed: 0, eta: 60, ratio: 0.2 } as unknown as DownloadRow;

describe('infoGroups / usenetGroups', () => {
	it('orders facts for downloading and seeding', () => {
		const dl = infoGroups(row, undefined)[0].rows;
		expect(dl[0].k).toBe('Downloaded');
		expect(dl.every((f) => f && f.v)).toBe(true);
		const seed = infoGroups({ ...row, progress: 1 } as DownloadRow, undefined)[0].rows;
		expect(seed[0].k).toBe('Share ratio');
	});
	it('prefers torrent detail over row fields', () => {
		const d = { downloader: 'q', requested_by: '', log: [], torrent: { size: 2000, downloaded: 2000, uploaded: 5, wasted: 0, speed: 0, up_speed: 0, eta: 0, ratio: 1.5, seeds: 1, seeds_total: 2, peers: 0, peers_total: 0, save_path: '/x' } };
		const g = infoGroups(row, d);
		expect(g[0].rows[0]).toMatchObject({ k: 'Share ratio', v: '1.50' });
		expect(g.flatMap((x) => x.rows).some((f) => f.k === 'Save path' && f.v === '/x')).toBe(true);
	});
	it('usenet shows Posted', () => {
		const g = usenetGroups({ ...row, posted_at: '2026-01-01T00:00:00Z' } as DownloadRow, undefined, undefined);
		expect(g.flatMap((x) => x.rows).some((f) => f.k === 'Posted')).toBe(true);
	});
	it('a failed download shows its whole reason, since the row cuts it and a tooltip is out of reach on touch', () => {
		const why = 'ERROR: Bad archive /data/downloads/Some.Show.S01E01.1080p/some.show.s01e01.part07.rar';
		const failed = { ...row, phase: 'failed', client_item: true, text: why } as unknown as DownloadRow;
		const facts = infoGroups(failed, undefined).flatMap((x) => x.rows);
		expect(facts.find((f) => f.k === 'Status')).toMatchObject({ wrap: true });
		expect(facts.find((f) => f.k === 'Status')?.v).toContain(why);
		const nzb = usenetGroups(failed, undefined, undefined).flatMap((x) => x.rows);
		expect(nzb.find((f) => f.k === 'Status')?.v).toContain(why);
		expect(infoGroups(row, undefined).flatMap((x) => x.rows).some((f) => f.k === 'Status')).toBe(false);
	});
});