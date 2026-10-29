import { describe, expect, it } from 'vitest';
import { sampleFiles, sampleLog, samplePeers } from './sampleDetails.ts';
import type { DownloadRow } from '../downloaders.ts';

const row = (o: Partial<DownloadRow>) => ({ id: 5, name: 'Night.Sample.2019.1080p.WEB-DL', size: 4 * 2 ** 30, progress: 0.5, phase: 'downloading', state: 'downloading', speed: 8_000_000, protocol: 'torrent', peers: 6, seeds: 2, ...o }) as DownloadRow;

describe('sample details', () => {
	it('torrent files add up to the size and fill in order', () => {
		const f = sampleFiles(row({}));
		expect(f.reduce((a, x) => a + x.size, 0)).toBe(4 * 2 ** 30);
		expect(f[0].done).toBeGreaterThan(0);
		expect(f[f.length - 1].done).toBe(0);
	});
	it('usenet files are a rar set with par2', () => {
		const f = sampleFiles(row({ protocol: 'usenet' }));
		expect(f.some((x) => x.name.endsWith('.rar'))).toBe(true);
		expect(f.some((x) => x.name.endsWith('.par2'))).toBe(true);
	});
	it('peers share the speed and mask addresses', () => {
		const p = samplePeers(row({}));
		expect(p).toHaveLength(6);
		expect(p.every((x) => x.ip.endsWith('.•.•'))).toBe(true);
		expect(Math.abs(p.reduce((a, x) => a + x.down, 0) - 8_000_000)).toBeLessThan(10);
	});
	it('a failed usenet item logs an error line', () => {
		expect(sampleLog(row({ protocol: 'usenet', phase: 'failed', state: 'failed' })).some((l) => l.bad)).toBe(true);
	});
});
