import { describe, expect, it } from 'vitest';
import { lifecycle, STAGE_WEIGHT } from './lifecycle.ts';

const ask = (state: string, title_id?: number) => ({ state, title_id }) as Parameters<typeof lifecycle>[0];

describe('lifecycle', () => {
	it('weights rise with each stage', () => {
		const w = Object.values(STAGE_WEIGHT);
		expect(w).toEqual([...w].sort((x, y) => x - y));
		expect(STAGE_WEIGHT.ready).toBe(100);
	});
	it('an ask waiting for an answer sits at the asked weight', () => {
		expect(lifecycle(ask('asked'))).toMatchObject({ stage: 'asked', pct: STAGE_WEIGHT.asked, tone: 'wait', label: 'Waiting for you' });
	});
	it('an approved ask with a title is being got', () => {
		expect(lifecycle(ask('approved', 7))).toMatchObject({ stage: 'getting', pct: STAGE_WEIGHT.getting, tone: 'go' });
	});
	it('download progress fills the getting-it span, never past it', () => {
		const lo = STAGE_WEIGHT.getting;
		const hi = STAGE_WEIGHT.getting_end;
		expect(lifecycle(ask('approved', 7), 0.5).pct).toBeCloseTo(lo + (hi - lo) * 0.5);
		expect(lifecycle(ask('approved', 7), 4).pct).toBe(hi);
		expect(lifecycle(ask('approved', 7), -1).pct).toBe(lo);
	});
	it('a broken progress number (NaN) reads as no progress, not an empty line', () => {
		expect(lifecycle(ask('approved', 7), Number.NaN)).toMatchObject({ pct: STAGE_WEIGHT.getting, label: 'Approved · getting it' });
	});
	it('in the Library is full and green', () => {
		expect(lifecycle(ask('available', 7))).toMatchObject({ stage: 'ready', pct: 100, tone: 'ok' });
	});
	it('turned down is a dim full line', () => {
		expect(lifecycle(ask('turned_down'))).toMatchObject({ stage: 'declined', pct: 100, tone: 'off' });
	});
	it('approved with no title failed where it stopped', () => {
		expect(lifecycle(ask('approved'))).toMatchObject({ stage: 'failed', pct: STAGE_WEIGHT.approved, tone: 'bad' });
	});
});
