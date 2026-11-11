import { healthLine, logDays, quietPeers, daysWord } from './panel-words.ts';
import { describe, expect, it } from 'vitest';

const now = new Date('2026-10-30T12:00:00Z').getTime();

describe('healthLine', () => {
	it('seeding', () => {
		expect(healthLine({ phase: 'done', state: 'seeding', progress: 1, ratio: 0.09, seeding_time: 15 * 86400 + 5 }, now)).toBe('Seeding · ratio 0.09 · 15 days');
	});
	it('seeding without seeding time uses the finish date', () => {
		expect(healthLine({ phase: 'done', state: 'seeding', ratio: 0, done_at: '2026-10-27T12:00:00Z' }, now)).toBe('Seeding · ratio 0.00 · 3 days');
	});
	it('downloading', () => {
		expect(healthLine({ phase: 'downloading', progress: 0.42, speed: 3_100_000, eta: 720 })).toBe('Downloading · 42% · 3.1 MB/s · 12m left');
	});
	it('paused and errored', () => {
		expect(healthLine({ phase: 'paused', progress: 0.5 })).toBe('Paused · 50%');
		expect(healthLine({ phase: 'failed', progress: 0.5, client_hint: 'errored' })).toBe('Failed · 50%');
		expect(healthLine({ phase: 'done', state: 'paused', ratio: 1 })).toBe('Done, paused · ratio 1.00');
	});
});

describe('quietPeers', () => {
	it('says when it last moved', () => {
		expect(quietPeers('2026-10-27T12:00:00Z', now)).toBe('0 connected now · last activity 3 days ago');
		expect(quietPeers(undefined, now)).toBe('0 connected now');
		expect(daysWord(5 * 3600)).toBe('5 h');
	});
});

describe('logDays', () => {
	it('groups newest first by day', () => {
		const g = logDays([
			{ at: '2026-10-15T10:00:00Z', kind: 'added', summary: 'Added' },
			{ at: '2026-10-30T09:00:00Z', kind: 'paused', summary: 'Paused' },
			{ at: '2026-10-30T11:00:00Z', kind: 'resumed', summary: 'Resumed' },
			{ at: 'bad', kind: 'x', summary: 'x' }
		], new Date(now));
		expect(g.map((d) => d.day)[0]).toBe('Today');
		expect(g[0].lines.map((l) => l.summary)).toEqual(['Resumed', 'Paused']);
		expect(g).toHaveLength(2);
		expect(logDays([])).toEqual([]);
	});
});

import { clientMark } from './panel-words.ts';
describe('clientMark', () => {
	it('marks each kind', () => {
		expect(clientMark('qbittorrent')).toEqual({ mark: 'qB', tone: 'qb' });
		expect(clientMark('nzbget').mark).toBe('NZB');
		expect(clientMark('rtorrent')).toEqual({ mark: 'rT', tone: 'rt' });
		expect(clientMark('other').mark).toBe('');
	});
});

import { swarmLine, swarmTotal } from './panel-words.ts';
describe('swarm counts', () => {
	it('shows only the connected number when the swarm total is 0 or smaller', () => {
		expect(swarmLine(1, 0)).toBe('1');
		expect(swarmLine(5, 3)).toBe('5');
		expect(swarmLine(3, 40)).toBe('3 of 40');
		expect(swarmTotal(1, 0)).toBe(0);
		expect(swarmTotal(2, 9)).toBe(9);
	});
});

describe('healthLine Usenet steps', () => {
	it('names the step after the download', () => {
		expect(healthLine({ phase: 'downloading', progress: 1, client_hint: 'repairing' })).toBe('Repairing');
		expect(healthLine({ phase: 'downloading', progress: 1, client_hint: 'script' })).toBe('Running script');
	});
});
