import { lifecycle, STAGE_WEIGHT, upcoming } from './lifecycle.ts';
import { describe, expect, it } from 'vitest';

const ask = (state: string, title_id?: number) => ({ state, title_id }) as Parameters<typeof lifecycle>[0];

describe('lifecycle live stages', () => {
	const a = ask('approved', 7);
	it('searching, downloading and importing sit inside the getting span', () => {
		const lo = STAGE_WEIGHT.getting;
		const hi = STAGE_WEIGHT.getting_end;
		const s = lifecycle(a, { phase: 'searching' });
		const d0 = lifecycle(a, { phase: 'downloading', progress: 0 });
		const d5 = lifecycle(a, { phase: 'downloading', progress: 0.5 });
		const d1 = lifecycle(a, { phase: 'downloading', progress: 1 });
		const i = lifecycle(a, { phase: 'importing' });
		expect(s).toMatchObject({ stage: 'searching', tone: 'go', label: 'Searching' });
		expect(d5).toMatchObject({ stage: 'downloading', tone: 'go', label: 'Downloading · 50%' });
		expect(i).toMatchObject({ stage: 'importing', tone: 'go', label: 'Importing' });
		const p = [s.pct, d0.pct, d5.pct, d1.pct, i.pct];
		expect(p).toEqual([...p].sort((x, y) => x - y));
		for (const x of p) {
			expect(x).toBeGreaterThan(lo);
			expect(x).toBeLessThanOrEqual(hi);
		}
	});
	it('a failed download shows Failed, and a live stage beats Upcoming', () => {
		expect(lifecycle(a, { phase: 'failed' })).toMatchObject({ stage: 'failed', tone: 'bad', label: 'Failed' });
		expect(lifecycle(a, { phase: 'searching', upcoming: 'Nov 3' }).label).toBe('Searching');
	});
});

describe('lifecycle', () => {
	it('weights rise with each stage', () => {
		const w = Object.values(STAGE_WEIGHT);
		expect(w).toEqual([...w].sort((x, y) => x - y));
		expect(STAGE_WEIGHT.ready).toBe(100);
	});
	it('an ask waiting for an answer sits at the asked weight', () => {
		expect(lifecycle(ask('asked'))).toMatchObject({ stage: 'asked', pct: STAGE_WEIGHT.asked, tone: 'wait', label: 'Waiting' });
	});
	it('an approved ask with a title is being got', () => {
		expect(lifecycle(ask('approved', 7))).toMatchObject({ stage: 'getting', pct: STAGE_WEIGHT.getting, tone: 'go', label: 'Processing' });
	});
	it('download progress fills the getting-it span, never past it', () => {
		const lo = STAGE_WEIGHT.getting;
		const hi = STAGE_WEIGHT.getting_end;
		expect(lifecycle(ask('approved', 7), { progress: 0.5 })).toMatchObject({ label: 'Processing' });
		expect(lifecycle(ask('approved', 7), { progress: 0.5 }).pct).toBeCloseTo(lo + (hi - lo) * 0.5);
		expect(lifecycle(ask('approved', 7), { progress: 4 }).pct).toBe(hi);
		expect(lifecycle(ask('approved', 7), { progress: -1 }).pct).toBe(lo);
	});
	it('a broken progress number (NaN) reads as no progress, not an empty line', () => {
		expect(lifecycle(ask('approved', 7), { progress: Number.NaN })).toMatchObject({ pct: STAGE_WEIGHT.getting, label: 'Processing' });
	});
	it('in the Library is full, green and Ready', () => {
		expect(lifecycle(ask('available', 7))).toMatchObject({ stage: 'ready', pct: 100, tone: 'ok', label: 'Ready' });
	});
	it('turned down is a dim full line', () => {
		expect(lifecycle(ask('turned_down'))).toMatchObject({ stage: 'declined', pct: 100, tone: 'off', label: 'Declined' });
	});
	it('approved with no title failed where it stopped, with its reason when known', () => {
		expect(lifecycle(ask('approved'))).toMatchObject({ stage: 'failed', pct: STAGE_WEIGHT.approved, tone: 'bad', label: 'Failed' });
		expect(lifecycle(ask('approved'), { reason: 'not found' })).toMatchObject({ label: 'Failed', reason: 'not found' });
	});
	it('a title not out yet is Upcoming, with its date when known', () => {
		expect(lifecycle(ask('approved', 7), { upcoming: 'Mar 3' })).toMatchObject({ stage: 'upcoming', tone: 'soon', label: 'Upcoming · Mar 3' });
		expect(lifecycle(ask('approved', 7), { upcoming: '' })).toMatchObject({ label: 'Upcoming' });
	});
	it('part of the asked seasons in the Library says how many', () => {
		expect(lifecycle(ask('approved', 7), { partial: { have: 2, total: 5 } })).toMatchObject({ stage: 'getting', tone: 'go', label: 'Processing · 3 left' });
		expect(lifecycle(ask('approved', 7), { partial: { have: 1, total: 2, unit: 'seasons' } }).label).toBe('Processing · 1 season left');
		expect(lifecycle(ask('approved', 7), { partial: { have: 3, total: 5, unit: 'albums' } }).label).toBe('Processing · 2 albums left');
		expect(lifecycle(ask('approved', 7), { partial: { have: 0, total: 5 } }).label).toBe('Processing');
		expect(lifecycle(ask('approved', 7), { partial: { have: 2, total: 2 } })).toMatchObject({ stage: 'ready', tone: 'ok', label: 'Ready' });
	});
	it('partial wins over upcoming, upcoming over progress', () => {
		expect(lifecycle(ask('approved', 7), { partial: { have: 1, total: 2 }, upcoming: 'Mar 3' }).label).toBe('Processing · 1 left');
		expect(lifecycle(ask('approved', 7), { upcoming: 'Mar 3', progress: 0.5 }).label).toBe('Upcoming · Mar 3');
	});
});

describe('ready with more to request', () => {
	it('names what came in and what is left to ask for', () => {
		const l = lifecycle(ask('approved', 7), { partial: { have: 2, total: 2, unit: 'albums' }, more: 5 });
		expect(l).toMatchObject({ stage: 'ready', label: 'Ready · 2 albums', more: '5 more albums available' });
		expect(lifecycle(ask('available', 7), { partial: { have: 1, total: 1, unit: 'seasons' } })).toMatchObject({ label: 'Ready · 1 season' });
		expect(lifecycle(ask('available', 7)).more).toBeUndefined();
	});
	it('unticked episodes or songs show when no whole part was skipped', () => {
		const show = { partial: { have: 3, total: 3, unit: 'seasons' as const } };
		expect(lifecycle(ask('approved', 7), { ...show, skipped: 4 }).more).toBe('4 more episodes available');
		expect(lifecycle(ask('approved', 7), { ...show, skipped: 1 }).more).toBe('1 more episode available');
		expect(lifecycle(ask('approved', 7), { ...show, skipped: 4, more: 1 }).more).toBe('1 more season available');
		expect(lifecycle(ask('approved', 7), { partial: { have: 2, total: 2, unit: 'albums' }, skipped: 6 }).more).toBe('6 more songs available');
	});
});

describe('upcoming', () => {
	const now = new Date('2026-10-30T12:00:00Z');
	const none = () => null;
	const movie = { kind: 'movie' } as const;
	it('a movie with every date ahead is upcoming from the earliest', () => {
		expect(upcoming(movie, { dates: { theaters: '2027-03-03', digital: '2027-05-01' } }, none, now)).toBe('2027-03-03');
	});
	it('a movie out anywhere, released, or with no dates is not', () => {
		expect(upcoming(movie, { dates: { theaters: '2026-01-01', digital: '2027-05-01' } }, none, now)).toBe('');
		expect(upcoming(movie, { dates: {}, status: 'Released' }, none, now)).toBe('');
		expect(upcoming(movie, { dates: {} }, none, now)).toBe('');
		expect(upcoming(movie, null, none, now)).toBe('');
	});
	it('a show is upcoming only when every asked season first airs later', () => {
		const air: Record<number, string> = { 3: '2027-01-10', 4: '2027-06-01', 2: '2025-01-01' };
		const f = (s: number) => air[s] ?? null;
		expect(upcoming({ kind: 'show', seasons: [3, 4] }, { dates: {} }, f, now)).toBe('2027-01-10');
		expect(upcoming({ kind: 'show', seasons: [2, 3] }, { dates: {} }, f, now)).toBe('');
		// A season with no known air day is not guessed.
		expect(upcoming({ kind: 'show', seasons: [3, 9] }, { dates: {} }, f, now)).toBe('');
	});
	it('the whole show reads its first air day', () => {
		expect(upcoming({ kind: 'show', seasons: [] }, { dates: { first_air: '2027-02-02' } }, none, now)).toBe('2027-02-02');
		expect(upcoming({ kind: 'show', future_only: true }, { dates: { first_air: '2027-02-02' } }, none, now)).toBe('');
	});
});
