import { barTone, facts, historyStatus, historyTotals, inHistory, matches, moving, totals } from './sections.ts';
import { describe, expect, it } from 'vitest';
import type { DownloadRow } from '../core/types.ts';

const row = (o: Partial<DownloadRow>): DownloadRow => ({
	id: 1, phase: 'downloading', state: 'downloading', label: '', text: '', progress: 0, speed: 0, active: false,
	name: 'Some.Release.1080p', title: 'Some', version: 'Main', downloader: 'SABnzbd', size: 0, updated_at: '2026-10-27T10:00:00Z', can_act: true, ...o
});

describe('sections', () => {
	it('puts done, seeding, failed and removed in History, the rest in the Queue', () => {
		const phases = ['waiting', 'paused', 'downloading', 'stalled', 'importing', 'needs_a_decision', 'done', 'failed', 'removed', 'vanished'];
		expect(phases.map((p) => inHistory(row({ phase: p })))).toEqual([false, false, false, false, false, false, true, true, true, true]);
	});
	it('colours bars by state', () => {
		expect(['downloading', 'waiting', 'paused', 'stalled', 'needs_a_decision'].map((p) => barTone(row({ phase: p })))).toEqual(['go', 'queued', 'held', 'held', 'bad']);
	});
});

describe('search', () => {
	it('matches title, Release name, Downloader and quality', () => {
		const x = row({ title: 'Arrival', name: 'Arrival.2016.2160p.DV.mkv', downloader: 'Basement qBit' });
		expect(['arriv', '2016.2160', 'basement', '2160p dv', ''].map((q) => matches(x, q))).toEqual([true, true, true, true, true]);
		expect(matches(x, 'nope')).toBe(false);
	});
	it('filters the list as you type and gives every row back once cleared', () => {
		const list = [row({ id: 1, title: 'Arrival', name: 'Arrival.2016.2160p.mkv' }), row({ id: 2, title: 'Dune', name: 'Dune.Part.Two.1080p.mkv' }), row({ id: 3, title: 'Severance', name: 'Severance.S02E01.720p.mkv' })];
		const find = (q: string) => list.filter((r) => matches(r, q)).map((r) => r.id);
		expect(find('a')).toEqual([1, 2, 3]);
		expect(find('ar')).toEqual([1, 2]);
		expect(find('arr')).toEqual([1]);
		expect(find('S02E01')).toEqual([3]);
		expect(find('part.two')).toEqual([2]);
		expect(find('  DUNE ')).toEqual([2]);
		expect(find('zzz')).toEqual([]);
		expect(find('')).toEqual([1, 2, 3]);
	});
});

describe('header strip', () => {
	it('gives speed only to rows taking in data', () => {
		const fast = { active: true, speed: 4194304 };
		expect(['downloading', 'waiting', 'paused', 'stalled', 'done'].map((p) => moving(row({ ...fast, phase: p })))).toEqual([true, false, false, false, false]);
		expect(totals([row({ ...fast, phase: 'waiting' }), row({ ...fast })]).speed).toBe(4194304);
	});
	it('adds speed and time left for the whole Queue', () => {
		const t = totals([row({ active: true, speed: 100, size: 1000, progress: 0.5 }), row({ size: 500 })]);
		expect(t).toEqual({ speed: 100, up: 0, left: 1000, eta: 10 });
		expect(totals([row({ active: true, speed: 100, up_speed: 30, size: 1000, progress: 0.5 })]).up).toBe(30);
		expect(totals([row({ size: 10 })]).eta).toBe(0);
	});

});

describe('history', () => {
	it('counts what finished today and this month', () => {
		const now = new Date(2026, 9, 27, 12);
		const rows = [
			row({ phase: 'done', size: 10, finished_at: new Date(2026, 9, 27, 9).toISOString() }),
			row({ phase: 'done', size: 5, finished_at: new Date(2026, 9, 3).toISOString() }),
			row({ phase: 'done', size: 7, finished_at: new Date(2026, 8, 3).toISOString() }),
			row({ phase: 'failed', size: 99, finished_at: new Date(2026, 9, 27).toISOString() })
		];
		expect(historyTotals(rows, now)).toEqual({ today: 10, month: 15 });
	});
	it('searches the title and the Release name', () => {
		expect(matches(row({}), 'release')).toBe(true);
		expect(matches(row({}), 'nope')).toBe(false);
		expect(matches(row({}), ' ')).toBe(true);
	});
	it('reads Done, or Failed with its reason', () => {
		expect(historyStatus(row({ phase: 'done' }), undefined)).toEqual({ text: 'Done', bad: false });
		expect(historyStatus(row({ phase: 'vanished' }), undefined)).toEqual({ text: 'Failed: removed outside HolyArrs', bad: true });
	});
	it('says an imported download is still in its Downloader while its clear waits', () => {
		const waiting = row({ phase: 'done', clear_waiting: { next_try: '2026-11-11T03:05:00Z', tries: 2, reason: 'busy' } });
		expect(historyStatus(waiting, undefined)).toEqual({ text: 'Imported', bad: false, still: 'Still in SABnzbd' });
		expect(historyStatus(row({ phase: 'failed', clear_waiting: { next_try: '', tries: 1 } }), undefined).still).toBeUndefined();
	});
});

describe('facts', () => {
	it('lists only the fields that have a value', () => {
		const f = facts(row({ size: 1048576, started_at: '2026-10-27T10:00:00Z' }), { ID: 1, State: 'done', CreatedAt: '', Plan: [{ source_file: '/dl/a.mkv', target_path: '/lib/Some (2020)/a.mkv' }] });
		expect(f.map((x) => x.label)).toEqual(['Name', 'Status', 'Size', 'Downloader', 'Path', 'Source', 'Added']);
	});
});
