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
	it('are All · Waiting · In progress · Problems · Done; All is the unfinished tabs together', () => {
		const tabs = tabsFor(rows, true);
		const c = counts(rows, tabs);
		expect(c.map((x) => `${x.label} ${x.count}`)).toEqual(['All 11', 'Waiting 2', 'In progress 5', 'Problems 4', 'Done 3']);
		expect(sorted(ids(pick(rows, tabs, 'active')))).toEqual(sorted(['waiting', 'progress', 'problems'].flatMap((k) => ids(pick(rows, tabs, k)))));
		expect(c.some((x) => x.label === 'Active')).toBe(false);
	});
	it('Done holds finished and seeding downloads; with All it covers every row', () => {
		const tabs = tabsFor(rows, true);
		expect(sorted(ids(pick(rows, tabs, 'done')))).toEqual([8, 9, 11]);
		expect(new Set([...ids(pick(rows, tabs, 'active')), ...ids(pick(rows, tabs, 'done'))]).size).toBe(rows.length);
		// A failed download is only under Problems, a stalled one counts as Downloading, a seeding one as Done.
		expect(ids(pick(rows, tabs, 'done'))).not.toContain(10);
	});
	it("each tab's sub-tabs split that tab", () => {
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
	it('sub-tabs: none on All or Done; Queued · Paused, Downloading · Importing, Failed · Review', () => {
		const tabs = tabsFor(rows, true);
		expect(tabs.map((t) => subCounts(rows, t).map((s) => `${s.label} ${s.count}`))).toEqual([
			[],
			['Queued 1', 'Paused 1'],
			['Downloading 2', 'Importing 3'],
			['Failed 3', 'Review 1'],
			[]
		]);
		expect(tabs.flatMap((t) => t.subs.map((x) => x.label))).not.toContain('Stalled');
		expect(tabs.flatMap((t) => t.subs.map((x) => x.label))).not.toContain('Seeding');
		// The parent pill itself means all of its parts, so there is no "All" sub-option.
		expect(tabs.flatMap((t) => t.subs.map((x) => x.label))).not.toContain('All');
		expect(pick(rows, tabs, 'waiting')).toEqual(pick(rows, tabs, 'waiting', ''));
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
		expect(rows.map((x) => statusWord(x))).toEqual(['Queued', 'Downloading', 'Importing', 'Stalled', 'Paused', 'Importing', 'Importing', 'Done', 'Seeding', 'Failed', 'Done', 'Failed', 'Review', 'Failed']);
	});
	it('never says Removed: removed outside HolyArrs reads Failed', () => {
		expect(statusWord({ phase: 'vanished', state: 'vanished', progress: 0.4 })).toBe('Failed');
		expect(rows.map((x) => statusWord(x))).not.toContain('Removed');
	});
});

describe('a failed copy that was replaced', () => {
	it('leaves Problems but still reads Failed', () => {
		const gone = { phase: 'vanished', state: 'vanished', progress: 0.2, replaced: true };
		const bad = { phase: 'failed', state: 'failed', progress: 0.2, replaced: true };
		expect(stageOf(gone)).toBe('done');
		expect(stageOf(bad)).toBe('done');
		expect(statusWord(bad)).toBe('Failed');
		expect(pick([gone, bad], TABS, 'problems')).toEqual([]);
		expect(pick([{ ...bad, replaced: false }], TABS, 'problems')).toHaveLength(1);
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
		expect(showOf('done.seeding')).toEqual({ show: 'done', sub: '' });
		expect(showOf('progress.stalled')).toEqual({ show: 'progress', sub: 'downloading' });
		expect(showOf('done.failed')).toEqual({ show: 'problems', sub: 'failed' });
		expect(showOf('seeding')).toEqual({ show: 'done', sub: '' });
	});
});
