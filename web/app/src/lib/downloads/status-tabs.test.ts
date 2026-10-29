import { describe, expect, it } from 'vitest';
import { ALL, ALL_HINT, TABS, counts, pick, showOf, stageOf, statusWord, subCounts, tabsFor } from './status-tabs.ts';

type R = { id: number; phase: string; state: string; progress: number; label: string };
const r = (id: number, phase: string, state = phase, progress = 0): R => ({ id, phase, state, progress, label: phase });

// One row of every phase the app knows, plus a Usenet job being unpacked
// (downloading at full progress) and a seeding torrent.
const rows: R[] = [
	r(1, 'waiting', 'queued'),
	r(2, 'downloading', 'downloading', 0.4),
	r(3, 'downloading', 'downloading', 1),
	r(4, 'stalled'),
	r(5, 'paused'),
	r(6, 'waiting_to_import', 'finished', 1),
	r(7, 'importing', 'finished', 1),
	r(8, 'done', 'finished', 1),
	r(9, 'done', 'seeding', 1),
	r(10, 'failed'),
	r(11, 'removed', 'cleaned', 1),
	r(12, 'vanished'),
	r(13, 'needs_a_decision', 'finished', 1),
	r(14, 'import_refused', 'finished', 1)
];
const ids = (xs: R[]) => xs.map((x) => x.id);
const sorted = (xs: number[]) => [...xs].sort((a, b) => a - b);

describe('stageOf', () => {
	it('puts every phase on one stage, the same for a torrent and a Usenet download', () => {
		expect(rows.map((x) => stageOf(x))).toEqual(['queued', 'downloading', 'importing', 'stalled', 'paused', 'importing', 'importing', 'done', 'seeding', 'failed', 'done', 'failed', 'decide', 'failed']);
	});
});

describe('the tabs', () => {
	it('are Active · Waiting · In progress · Problems · Done; Active is the three unfinished tabs together', () => {
		const tabs = tabsFor(rows, true);
		const c = counts(rows, tabs);
		expect(c.map((x) => `${x.label} ${x.count}`)).toEqual(['Active 11', 'Waiting 2', 'In progress 5', 'Problems 4', 'Done 6']);
		expect(sorted(ids(pick(rows, tabs, 'active')))).toEqual(sorted(['waiting', 'progress', 'problems'].flatMap((k) => ids(pick(rows, tabs, k)))));
		expect(c.find((x) => x.key === 'done')).toMatchObject({ seeding: 1 });
		expect(c.some((x) => x.label === 'All')).toBe(false);
	});
	it('Done holds finished and failed downloads; with Active it covers every row', () => {
		const tabs = tabsFor(rows, true);
		expect(sorted(ids(pick(rows, tabs, 'done')))).toEqual([8, 9, 10, 11, 12, 14]);
		expect(new Set([...ids(pick(rows, tabs, 'active')), ...ids(pick(rows, tabs, 'done'))]).size).toBe(rows.length);
	});
	it("each tab's sub-tabs split that tab, with or without the Seeding sub-tab", () => {
		for (const torrent of [true, false]) {
			for (const data of [rows, rows.filter((x) => x.state !== 'seeding')]) {
				const tabs = tabsFor(data, torrent);
				for (const t of tabs.filter((x) => x.subs.length)) {
					const inTab = pick(data, tabs, t.key);
					expect(subCounts(data, t).reduce((n, s) => n + s.count, 0)).toBe(inTab.length);
					expect(sorted(t.subs.flatMap((s) => ids(pick(data, tabs, t.key, s.key))))).toEqual(sorted(ids(inTab)));
				}
			}
		}
	});
	it('sub-tabs: none on Active; Queued · Paused, Downloading · Stalled · Importing, Failed · Review, Done · Seeding · Failed', () => {
		const tabs = tabsFor(rows, true);
		expect(tabs.map((t) => subCounts(rows, t).map((s) => `${s.label} ${s.count}`))).toEqual([
			[],
			['Queued 1', 'Paused 1'],
			['Downloading 1', 'Stalled 1', 'Importing 3'],
			['Failed 3', 'Review 1'],
			['Done 2', 'Seeding 1', 'Failed 3']
		]);
	});
	it('the Seeding sub-tab shows only with a torrent Downloader or a seeding row', () => {
		const calm = rows.filter((x) => x.state !== 'seeding');
		const doneSubs = (data: R[], torrent: boolean) => tabsFor(data, torrent).find((t) => t.key === 'done')?.subs.map((s) => s.key);
		expect(doneSubs(calm, false)).toEqual(['done', 'failed']);
		expect(doneSubs(calm, true)).toEqual(['done', 'seeding', 'failed']);
		expect(doneSubs(rows, false)).toEqual(['done', 'seeding', 'failed']);
	});
	it('every tab and sub-tab has a short hint and an empty-state title', () => {
		expect(ALL_HINT).toMatch(/^[A-Z].+\.$/);
		for (const t of TABS) {
			expect(t.hint).toMatch(/^[A-Z].{8,90}\.$/);
			expect(t.empty).toMatch(/^(Nothing|No) .+\.$/);
		}
		for (const s of TABS.flatMap((t) => t.subs)) {
			expect(s.hint).toMatch(/^[A-Z].{8,90}\.$/);
			expect(s.empty).toMatch(/^Nothing .+\.$/);
		}
	});
});

describe('statusWord', () => {
	it('follows the sub-tab words', () => {
		expect(rows.map((x) => statusWord(x))).toEqual(['Queued', 'Downloading', 'Importing', 'Stalled', 'Paused', 'Importing', 'Importing', 'Done', 'Seeding', 'Failed', 'Removed', 'Failed', 'Review', 'Failed']);
	});
});

describe('showOf', () => {
	it('reads ?show= for both levels, safely', () => {
		expect(showOf('problems')).toEqual({ show: 'problems', sub: '' });
		expect(showOf('problems.decide')).toEqual({ show: 'problems', sub: 'decide' });
		expect(showOf('progress.nope')).toEqual({ show: 'progress', sub: '' });
		expect(showOf('finished')).toEqual({ show: ALL, sub: '' });
		expect(showOf(null)).toEqual({ show: ALL, sub: '' });
		expect(showOf('all')).toEqual({ show: 'active', sub: '' });
		expect(showOf('done.seeding')).toEqual({ show: 'done', sub: 'seeding' });
	});
});
