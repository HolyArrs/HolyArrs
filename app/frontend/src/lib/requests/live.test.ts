import { liveByTitle, liveFacts, partialFacts, realAlbum, stepFacts } from './live.ts';
import { lifecycle } from './lifecycle.ts';
import { describe, expect, it } from 'vitest';

const dl = (title_id: number, phase: string, progress = 0) => ({ title_id, phase, progress });
const se = (title_id: number, state = 'running', kind = 'search_slot') => ({ title_id, state, kind }) as Parameters<typeof liveByTitle>[1][number];

describe('liveByTitle', () => {
	it('maps phases to downloading, importing and failed', () => {
		const m = liveByTitle([dl(1, 'downloading', 0.4), dl(2, 'importing', 1), dl(3, 'failed'), dl(4, 'done', 1), dl(5, 'waiting')], []);
		expect(liveFacts(m.get(1))).toEqual({ phase: 'downloading', progress: 0.4 });
		expect(liveFacts(m.get(2))).toEqual({ phase: 'importing', progress: 1 });
		expect(liveFacts(m.get(3))).toEqual({ phase: 'failed' });
		expect(m.has(4)).toBe(false);
		expect(liveFacts(m.get(5)).phase).toBe('downloading');
	});
	it('running searches count, finished ones and feed checks do not', () => {
		const m = liveByTitle([], [se(1), se(2, 'done'), se(3, 'queued'), se(4, 'running', 'feed_check')]);
		expect(liveFacts(m.get(1))).toEqual({ phase: 'searching' });
		expect(m.has(2)).toBe(false);
		expect(m.has(3)).toBe(true);
		expect(m.has(4)).toBe(false);
	});
	it('a copy still moving beats a failed one, and a search beats a failed copy', () => {
		const m = liveByTitle([dl(1, 'failed'), dl(1, 'downloading', 0.2), dl(2, 'failed')], [se(2)]);
		expect(liveFacts(m.get(1)).phase).toBe('downloading');
		expect(liveFacts(m.get(2)).phase).toBe('searching');
	});
	it('a failed copy passes on the server reason', () => {
		const m = liveByTitle([{ ...dl(1, 'failed'), text: 'No progress for 6 hours.' }, { ...dl(2, 'failed'), text: ' ' }], []);
		expect(liveFacts(m.get(1))).toEqual({ phase: 'failed', reason: 'No progress for 6 hours.' });
		expect(liveFacts(m.get(2))).toEqual({ phase: 'failed' });
	});
	it('nothing known gives no facts', () => {
		expect(liveFacts(undefined)).toEqual({});
	});
});

describe('real parts in "more available" (#135)', () => {
	it('counts only real albums the ask did not cover', () => {
		const parts = [
			{ key: 'a', complete: true, real: true },
			{ key: 'b', complete: false, real: true },
			{ key: 'single', complete: false, real: false },
			{ key: 'remix', complete: false, real: false }
		];
		expect(partialFacts('albums', parts, ['a']).more).toBe(1);
	});
	it('realAlbum drops singles, secondary types and bootleg titles', () => {
		expect(realAlbum({ type: 'Album', secondary_types: [], name: 'Views' })).toBe(true);
		expect(realAlbum({ type: 'Single', secondary_types: [], name: 'Hotline Bling' })).toBe(false);
		expect(realAlbum({ type: 'Album', secondary_types: ['Remix'], name: 'X' })).toBe(false);
		expect(realAlbum({ type: 'Album', secondary_types: [], name: 'Unreleased Bootleg Vol. 2' })).toBe(false);
	});
});

describe('episodes or songs left unticked (#135)', () => {
	const seasons = [
		{ key: 1, complete: true, real: true, size: 10, have: 10 },
		{ key: 2, complete: false, real: true, size: 8, have: 3 },
		{ key: 3, complete: false, real: true, size: 6, have: 0 }
	];
	it('counts the episodes left out of a season asked in part', () => {
		const f = partialFacts('seasons', seasons, [1], [{ key: 2, count: 3 }]);
		expect(f).toEqual({ partial: { have: 2, total: 2, unit: 'seasons' }, more: 1, skipped: 5 });
		expect(lifecycle({ state: 'available', title_id: 7 }, f).more).toBe('1 more season available');
		const all = partialFacts('seasons', seasons, [1, 3], [{ key: 2, count: 3 }]);
		expect(lifecycle({ state: 'approved', title_id: 7 }, all).label).toBe('Processing · 1 season left');
	});
	it('a season with picks is asked, not more: Ready says the unticked episodes', () => {
		const f = partialFacts('seasons', seasons.slice(0, 2), [1], [{ key: 2, count: 3 }]);
		expect(f.more).toBe(0);
		expect(lifecycle({ state: 'approved', title_id: 7 }, f)).toMatchObject({ stage: 'ready', label: 'Ready · 2 seasons', more: '5 more episodes available' });
	});
	it('picks only (no whole season) count the picked episodes (#139)', () => {
		const f = partialFacts('seasons', seasons, [], [{ key: 3, count: 2 }]);
		expect(f).toEqual({ partial: { have: 0, total: 2, unit: 'episodes' }, more: 2, skipped: 4 });
		expect(lifecycle({ state: 'approved', title_id: 7 }, f).label).toBe('Processing');
		// One episode asked and in: "Ready · 1 episode"; more counts whole seasons.
		const one = partialFacts('seasons', [{ key: 1, complete: false, real: true, size: 9, have: 1 }, { key: 2, complete: false, real: true, size: 9, have: 0 }], [], [{ key: 1, count: 1 }]);
		expect(lifecycle({ state: 'available', title_id: 7 }, one)).toMatchObject({ label: 'Ready · 1 episode', more: '1 more season available' });
		// Files on disk are counted per season, unpicked ones too, so never Ready early from them.
		const disk = partialFacts('seasons', [{ key: 2, complete: false, real: true, size: 8, have: 5 }], [], [{ key: 2, count: 3 }]);
		expect(lifecycle({ state: 'approved', title_id: 7 }, disk).label).toBe('Processing');
	});
	it('a part asked whole and in part counts as whole', () => {
		expect(partialFacts('seasons', seasons, [2], [{ key: 2, count: 3 }]).skipped).toBeUndefined();
	});
	it('counts the songs left out of an album', () => {
		const albums = [
			{ key: 'a', complete: false, real: true, size: 12, have: 9 },
			{ key: 'b', complete: true, real: true, size: 10, have: 10 }
		];
		const f = partialFacts('albums', albums, ['b'], [{ key: 'a', count: 9 }]);
		expect(f).toEqual({ partial: { have: 2, total: 2, unit: 'albums' }, more: 0, skipped: 3 });
		expect(lifecycle({ state: 'available', title_id: 7 }, f).more).toBe('3 more songs available');
	});
	it('no sizes known: nothing skipped, nothing guessed', () => {
		const f = partialFacts('albums', [{ key: 'a', complete: false }], [], [{ key: 'a', count: 2 }]);
		expect(f).toEqual({ partial: { have: 0, total: 1, unit: 'albums' }, more: 0 });
	});
});

describe('a download waiting on a person', () => {
	it('shows Needs a decision instead of Processing', () => {
		for (const phase of ['needs_a_decision', 'import_refused']) {
			const t = liveByTitle([dl(7, phase, 1)], []).get(7);
			expect(liveFacts(t)).toEqual({ phase: 'stuck', progress: 1 });
		}
	});
	it('a copy still downloading wins over a stuck one', () => {
		expect(liveByTitle([dl(7, 'needs_a_decision', 1), dl(7, 'downloading', 0.4)], []).get(7)?.download?.phase).toBe('downloading');
	});
});

describe('the server step when the page has no live detail (#139)', () => {
	const approved = { state: 'approved' as const, title_id: 7 };
	it('names each step, never a made-up percent', () => {
		expect(lifecycle(approved, stepFacts({ kind: 'searching' })).label).toBe('Searching');
		expect(lifecycle(approved, stepFacts({ kind: 'downloading' })).label).toBe('Downloading');
		expect(lifecycle(approved, stepFacts({ kind: 'importing' })).label).toBe('Importing');
		expect(lifecycle(approved, stepFacts({ kind: 'waiting_on_admin' })).label).toBe('Needs a decision');
		expect(lifecycle(approved, stepFacts({ kind: 'not_out' })).label).toBe('Upcoming');
		expect(lifecycle(approved, stepFacts(undefined)).label).toBe('Processing');
	});
	it('waiting says why and when it last looked, instead of a bare Processing', () => {
		const life = lifecycle(approved, stepFacts({ kind: 'waiting', text: 'no Release passed the Pick rules (2 found, 2 refused)', searched_at: '2026-11-03T08:00:00Z' }));
		expect(life).toMatchObject({ label: 'Looking for a release', tone: 'wait', reason: 'no Release passed the Pick rules (2 found, 2 refused)', searchedAt: '2026-11-03T08:00:00Z' });
	});
});
