import { ALL, ALL_HINT, TABS, counts, emptyOf, pick, showOf, stageOf, statusWord, subCounts, tabsFor } from './status-tabs.ts';
import { describe, expect, it } from 'vitest';

type R = { id: number; phase: string; state: string; progress: number; label: string };
const r = (id: number, phase: string, state = phase, progress = 0): R => ({ id, phase, state, progress, label: phase });

// One row of every phase the app knows, plus a Usenet job being unpacked (downloading at full progress) and a seeding torrent.
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
	it('are All · Waiting · In progress · Problems · Done; All is every row, Done included', () => {
		const tabs = tabsFor(rows, true);
		const c = counts(rows, tabs);
		expect(c.map((x) => `${x.label} ${x.count}`)).toEqual(['All 14', 'Waiting 2', 'In progress 5', 'Problems 4', 'Done 3']);
		expect(sorted(ids(pick(rows, tabs, 'active')))).toEqual(sorted(ids(rows)));
		// Only seeding torrents: All counts them, so it is never 0.
		const seeding = [r(1, 'done', 'seeding', 1), r(2, 'done', 'seeding', 1)];
		expect(counts(seeding, tabs)[0].count).toBe(2);
		expect(c.some((x) => x.label === 'Active')).toBe(false);
	});
	it('Done holds finished and seeding downloads; with All it covers every row', () => {
		const tabs = tabsFor(rows, true);
		expect(sorted(ids(pick(rows, tabs, 'done')))).toEqual([8, 9, 11]);
		expect(ids(pick(rows, tabs, 'active')).length).toBe(rows.length);
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
	it('sub-tabs: none on All or Done; Queued · Paused, Downloading · Checking · Importing, Failed · Review', () => {
		const tabs = tabsFor(rows, true);
		expect(tabs.map((t) => subCounts(rows, t).map((s) => `${s.label} ${s.count}`))).toEqual([
			[],
			['Queued 1', 'Paused 1'],
			['Downloading 2', 'Checking 0', 'Importing 3'],
			['Failed 3', 'Review 1'],
			[]
		]);
		expect(tabs.flatMap((t) => t.subs.map((x) => x.label))).not.toContain('Stalled');
		expect(tabs.flatMap((t) => t.subs.map((x) => x.label))).not.toContain('Seeding');
		// The parent pill means all of its parts, so there is no "All" sub-option.
		expect(tabs.flatMap((t) => t.subs.map((x) => x.label))).not.toContain('All');
		expect(pick(rows, tabs, 'waiting')).toEqual(pick(rows, tabs, 'waiting', ''));
	});
	it('every tab and sub-tab has a short hint and an empty-state title', () => {
		expect(ALL_HINT).toMatch(/^[A-Z].+\.$/);
		for (const t of TABS) {
			expect(t.hint).toMatch(/^[A-Z].{8,90}\.$/);
			expect(t.empty).toMatch(/^[A-Z][^.]{4,40}$/); // A short heading with no full stop.
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

describe('emptyOf', () => {
	it('gives each tab its own calm card', () => {
		expect(emptyOf('active')).toMatchObject({ title: 'No downloads yet', text: 'Anything HolyArrs or your Downloader is fetching shows up here.' });
		expect(emptyOf('active').action).toBeUndefined();
		expect(emptyOf('problems')).toMatchObject({ title: 'All clear', glyph: 'check' });
		expect(emptyOf('done').title).toBe('Nothing finished yet');
		expect(emptyOf('waiting', 'paused').title).toBe('Nothing is paused');
	});
	it('a search or a kind filter with no hits offers the way back', () => {
		expect(emptyOf('active', '', '  dune ')).toMatchObject({ title: 'No matches for “dune”', action: { act: 'clear' } });
		expect(emptyOf('done', '', '', 'artist')).toMatchObject({ title: 'No music downloads', action: { act: 'all' } });
		expect(emptyOf('active', '', '', '', TABS, 'usenet').title).toBe('No Usenet downloads');
	});
});

import { statusHint as hintOf, shortReason, stepLine } from './status-tabs.ts';
describe('short reason', () => {
	it('drops the status code and the Downloader name', () => {
		expect(shortReason('Deleted in NZBGet by hand (DELETED/MANUAL)')).toBe('Deleted by hand');
		expect(shortReason('Marked bad in NZBGet (FAILURE/BAD)')).toBe('Marked bad');
		expect(shortReason('136 articles were missing')).toBe('136 articles missing');
	});
});
describe('step line', () => {
	it('is two words at most and never repeats the status word', () => {
		expect(stepLine({ client_hint: 'unpacking' }, 'unpacking')).toBe('Unpacking');
		expect(stepLine({ client_hint: 'script' }, 'unpacking')).toBe('Running script');
		expect(stepLine({}, 'up next')).toBe('Up next');
		expect(stepLine({}, 'placing files')).toBe('Placing files');
		expect(stepLine({}, 'import failed: no file could be placed')).toBe('Import failed');
		expect(stepLine({}, 'removed outside HolyArrs')).toBe('Removed outside');
		expect(stepLine({}, 'the files did not match')).toBe('Files did not match');
		expect(stepLine({}, 'ratio 1.23')).toBe('Ratio 1.23');
		expect(stepLine({}, '')).toBe('');
	});
});
describe('client hints', () => {
	const base = { progress: 0.5, state: 'queued' } as const;
	it('puts checking and moving under In progress with one-word statuses', () => {
		const r = { ...base, phase: 'waiting', client_hint: 'checking' as const };
		expect(stageOf(r)).toBe('checking');
		expect(statusWord(r)).toBe('Checking');
		expect(statusWord({ ...r, client_hint: 'moving' as const })).toBe('Moving');
		expect(pick([r], TABS, 'progress', 'checking')).toHaveLength(1);
		expect(subCounts([r], TABS.find((t) => t.key === 'progress')).find((s) => s.key === 'checking')?.count).toBe(1);
	});
	it('says Failed for client errors, with the cause on the step line, and keeps idle seeding under Done', () => {
		expect(statusWord({ ...base, phase: 'failed', client_hint: 'errored' })).toBe('Failed');
		expect(stepLine({ client_hint: 'errored' }, '')).toBe('Downloader error');
		expect(statusWord({ ...base, phase: 'failed', client_hint: 'missing' })).toBe('Failed');
		expect(stepLine({ client_hint: 'missing' }, '')).toBe('Files missing');
		const idle = { progress: 1, phase: 'done', state: 'seeding', client_hint: 'idle_seed' as const };
		expect(statusWord(idle)).toBe('Seeding');
		expect(pick([idle], TABS, 'done')).toHaveLength(1);
		expect(hintOf(idle)).toMatch(/Nobody/);
		expect(statusWord({ ...base, phase: 'stalled' })).toBe('Stalled');
		expect(pick([{ ...base, phase: 'stalled' }], TABS, 'progress', 'downloading')).toHaveLength(1);
	});
});

import { packLine } from './status-tabs.ts';
describe('a season pack taken a few episodes at a time', () => {
	const pack = { from: 1, to: 2, taken: 2, total: 9 };
	it('names the episodes switched on', () => {
		expect(packLine({ pack })).toBe('Season pack: episodes 1–2 (2 of 9 taken)');
		expect(packLine({ pack: { from: 3, to: 3, taken: 1, total: 9 } })).toBe('Season pack: episode 3 (1 of 9 taken)');
		expect(packLine({})).toBe('');
	});
	it('reads Waiting for next episodes once the taken ones are in', () => {
		expect(statusWord({ phase: 'done', state: 'seeding', progress: 1, pack })).toBe('Waiting for next episodes');
		expect(statusWord({ phase: 'done', state: 'finished', progress: 1, pack })).toBe('Waiting for next episodes');
		expect(stageOf({ phase: 'done', state: 'seeding', progress: 1, pack })).toBe('seeding');
	});
	it('reads as usual while downloading or once every episode is taken', () => {
		expect(statusWord({ phase: 'downloading', state: 'downloading', progress: 0.3, pack })).toBe('Downloading');
		expect(statusWord({ phase: 'done', state: 'seeding', progress: 1, pack: { from: 1, to: 9, taken: 9, total: 9 } })).toBe('Seeding');
	});
});
