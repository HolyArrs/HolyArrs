import { describe, expect, it } from 'vitest';
import { barOf, flipFrom, loadPosterDates, loadPosterDetail, needsAttention, posterNote, posterYear, savePosterDates, savePosterDetail, tiltFor } from './poster.ts';
import type { LibState } from './library-state.ts';

const st = (key: LibState['key'], percent?: number): LibState => ({ key, label: key, tone: 'ok', icon: 'owned', pill: 'in_library', rank: 0, ...(percent !== undefined ? { percent } : {}) });

describe('needsAttention', () => {
	it('stays quiet for healthy titles', () => {
		for (const k of ['in_library', 'up_to_date', 'ended', 'specials_only', 'approved', 'not_kept', 'declined'] as const) expect(needsAttention(st(k))).toBe(false);
		expect(needsAttention(null)).toBe(false);
	});
	it('flags every problem and in-progress state', () => {
		for (const k of ['failed', 'needs_decision', 'stalled', 'missing_file', 'missing', 'better_wanted', 'paused', 'downloading', 'queued', 'waiting_approval', 'not_out_yet'] as const) expect(needsAttention(st(k))).toBe(true);
	});
});

describe('barOf', () => {
	it('is a full green line when healthy', () => expect(barOf(st('in_library'), 0, 0)).toEqual({ tone: 'ok', fill: 1 }));
	it('fills by download percent', () => expect(barOf(st('downloading', 42), 0, 0)).toEqual({ tone: 'busy', fill: 0.42 }));
	it('shows the share in for missing episodes', () => expect(barOf(st('missing'), 3, 4)).toEqual({ tone: 'gap', fill: 0.75 }));
	it('is red for failures', () => expect(barOf(st('failed'), 0, 0).tone).toBe('fail'));
	it('is grey for not out yet', () => expect(barOf(st('not_out_yet'), 0, 0).tone).toBe('idle'));
	it('treats no state (a quiet show) as complete', () => expect(barOf(null, 10, 10)).toEqual({ tone: 'ok', fill: 1 }));
});

describe('tiltFor', () => {
	it('is flat at the centre', () => expect(tiltFor(50, 75, 100, 150)).toEqual({ rx: 0, ry: 0 }));
	it('leans toward the top right corner at most max degrees', () => expect(tiltFor(100, 0, 100, 150)).toEqual({ rx: 4, ry: 4 }));
	it('leans toward the bottom left', () => expect(tiltFor(0, 150, 100, 150, 4)).toEqual({ rx: -4, ry: -4 }));
	it('clamps outside the box', () => expect(tiltFor(500, -40, 100, 150)).toEqual({ rx: 4, ry: 4 }));
	it('is flat for an empty box', () => expect(tiltFor(10, 10, 0, 0)).toEqual({ rx: 0, ry: 0 }));
});

describe('poster detail', () => {
	it('defaults to title and problems, and remembers per member', () => {
		const m = new Map<string, string>();
		const store = { getItem: (k: string) => m.get(k) ?? null, setItem: (k: string, v: string) => void m.set(k, v) };
		expect(loadPosterDetail(1, store)).toBe('status');
		savePosterDetail(1, 'full', store);
		expect(loadPosterDetail(1, store)).toBe('full');
		expect(loadPosterDetail(2, store)).toBe('status');
	});
});

describe('flipFrom', () => {
	it('is null when nothing moved', () => expect(flipFrom({ left: 0, top: 0, width: 100 }, { left: 0, top: 0, width: 100 })).toBeNull());
	it('inverts a move and a resize', () => expect(flipFrom({ left: 10, top: 300, width: 120 }, { left: 200, top: 0, width: 160 })).toBe('translate(-190px, 300px) scale(0.75)'));
});

describe('poster dates', () => {
	it('shows the year until full dates are on', () => {
		const m = { kind: 'movie', year: 2026, digital_date: '2026-03-14', cinema_date: '2026-01-02' };
		expect(posterYear(m, false)).toBe('2026');
		expect(posterYear(m, true)).toMatch(/2026/);
		expect(posterYear(m, true)).not.toMatch(/14/);
		expect(posterYear({ kind: 'movie', year: 2010, cinema_date: '2010-07-15', digital_date: '2020-08-13' }, true)).toMatch(/2010/);
		expect(posterYear({ kind: 'movie', year: 2027, cinema_date: '2027-05-01' }, true)).toMatch(/2027/);
		expect(posterYear({ kind: 'movie', year: 2027 }, true)).toBe('2027');
		expect(posterYear({ kind: 'movie' }, true)).toBe('');
		expect(posterYear({ kind: 'show', year: 2020, digital_date: '2020-02-02' }, true)).toBe('2020');
		expect(posterYear({ kind: 'show', year: 2020, show: { next_air_date: '2026-12-03T00:00:00Z', last_air_date: '2026-11-26T00:00:00Z' } }, true)).toBe('2020');
		expect(posterNote({ kind: 'show', show: { next_air_date: '2026-12-03T00:00:00Z' } }, false)).toBe('');
		expect(posterNote({ kind: 'show', year: 2020, show: { next_air_date: '2026-12-03T00:00:00Z', last_air_date: '2026-11-26T00:00:00Z' } }, true)).toMatch(/^Next episode .*3/);
		expect(posterNote({ kind: 'show', year: 2020, show: { last_air_date: '2026-11-26T00:00:00Z' } }, true)).toMatch(/^Last aired .*26/);
		expect(posterNote({ kind: 'artist', year: 1999, artist: { last_release: '2024-05-17T00:00:00Z' } }, true)).toMatch(/^Newest release .*2024/);
		expect(posterNote({ kind: 'artist', year: 1999, artist: {} }, true)).toBe('');
		expect(posterNote({ kind: 'movie', cinema_date: '2026-01-02' }, true)).toBe('');
	});
	it('remembers the choice per person', () => {
		const m = new Map<string, string>();
		const store = { getItem: (k: string) => m.get(k) ?? null, setItem: (k: string, v: string) => void m.set(k, v) };
		expect(loadPosterDates(1, store)).toBe(false);
		savePosterDates(1, true, store);
		expect(loadPosterDates(1, store)).toBe(true);
		expect(loadPosterDates(2, store)).toBe(false);
		expect(loadPosterDates(1, { getItem: () => { throw new Error('blocked'); }, setItem: () => {} })).toBe(false);
	});
});
