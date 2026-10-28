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
	it('are All · Waiting · In progress · Done · Problems, and split All exactly', () => {
		const c = counts(rows, tabsFor(rows, true));
		expect(c.map((x) => `${x.label} ${x.count}`)).toEqual(['All 14', 'Waiting 2', 'In progress 5', 'Done 3', 'Problems 4']);
		expect(c.slice(1).reduce((n, x) => n + x.count, 0)).toBe(rows.length);
		expect(c.find((x) => x.key === 'done')).toMatchObject({ seeding: 1 });
	});
	it('each tab\'s sub-tabs split that tab, with or without the Seeding sub-tab', () => {
		for (const torrent of [true, false]) {
			for (const data of [rows, rows.filter((x) => x.state !== 'seeding')]) {
				const tabs = tabsFor(data, torrent);
				expect(sorted(tabs.flatMap((t) => ids(pick(data, tabs, t.key))))).toEqual(ids(data));
				for (const t of tabs) {
					const inTab = pick(data, tabs, t.key);
					expect(subCounts(data, t).reduce((n, s) => n + s.count, 0)).toBe(inTab.length);
					expect(sorted(t.subs.flatMap((s) => ids(pick(data, tabs, t.key, s.key))))).toEqual(ids(inTab));
				}
			}
		}
	});
	it('sub-tabs: Queued · Paused, Downloading · Importing (stalled under Downloading), Done · Seeding, Failed · Needs a decision; no All sub-tab', () => {
		const tabs = tabsFor(rows, true);
		expect(tabs.map((t) => subCounts(rows, t).map((s) => `${s.label} ${s.count}`))).toEqual([
			['Queued 1', 'Paused 1'],
			['Downloading 2', 'Importing 3'],
			['Done 2', 'Seeding 1'],
			['Failed 3', 'Needs a decision 1']
		]);
	});
	it('the Seeding sub-tab shows only with a torrent Downloader or a seeding row', () => {
		const calm = rows.filter((x) => x.state !== 'seeding');
		const doneSubs = (data: R[], torrent: boolean) => tabsFor(data, torrent).find((t) => t.key === 'done')?.subs.map((s) => s.key);
		expect(doneSubs(calm, false)).toEqual(['done']);
		expect(doneSubs(calm, true)).toEqual(['done', 'seeding']);
		expect(doneSubs(rows, false)).toEqual(['done', 'seeding']);
	});
	it('every tab and sub-tab has a short hint and an empty-state title', () => {
		expect(ALL_HINT).toMatch(/^[A-Z].+\.$/);
		expect(TABS.map((t) => t.hint)).toEqual([
			"In your Downloader's line, or paused by you.",
			'On its way: downloading now, or being checked and filed into your Library.',
			'In your Library. Torrents still sharing with others count here.',
			'Stuck, failed, or waiting for you to decide.'
		]);
		expect(TABS.map((t) => t.empty)).toEqual(['Nothing waiting.', 'Nothing on its way right now.', 'Nothing finished yet.', 'No problems. Everything is moving.']);
		for (const s of TABS.flatMap((t) => t.subs)) {
			expect(s.hint).toMatch(/^[A-Z].{8,90}\.$/);
			expect(s.empty).toMatch(/^Nothing .+\.$/);
		}
	});
});

describe('statusWord', () => {
	it('follows the sub-tab words', () => {
		expect(rows.map((x) => statusWord(x))).toEqual(['Queued', 'Downloading', 'Importing', 'Stalled', 'Paused', 'Importing', 'Importing', 'Done', 'Seeding', 'Failed', 'Removed', 'Failed', 'Needs a decision', 'Failed']);
	});
});

describe('showOf', () => {
	it('reads ?show= for both levels, safely', () => {
		expect(showOf('problems')).toEqual({ show: 'problems', sub: '' });
		expect(showOf('problems.decide')).toEqual({ show: 'problems', sub: 'decide' });
		expect(showOf('progress.nope')).toEqual({ show: 'progress', sub: '' });
		expect(showOf('finished')).toEqual({ show: ALL, sub: '' });
		expect(showOf(null)).toEqual({ show: ALL, sub: '' });
	});
});
