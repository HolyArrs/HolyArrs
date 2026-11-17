import { describe, expect, it } from 'vitest';
import { nextCheckIn, outageDismiss, outageShown, outageStart, outageStep } from './outage.ts';

function goDown(at: number, from = outageStart()) {
	let s = from;
	for (const t of [at, at + 30_000, at + 60_000]) s = outageStep(s, false, t).state;
	return s;
}

describe('outage', () => {
	it('ignores one failed check', () => {
		const r = outageStep(outageStart(), false, 1000);
		expect(r.event).toBeNull();
		expect(outageShown(r.state)).toBe(false);
	});

	it('a short run of failures stays quiet', () => {
		let s = outageStart();
		for (const at of [0, 10_000, 20_000, 30_000]) {
			const r = outageStep(s, false, at);
			expect(r.event).toBeNull();
			s = r.state;
		}
		expect(outageShown(s)).toBe(false);
	});

	it('goes down after 3 failures spread over 60 s', () => {
		let s = outageStart();
		for (const at of [0, 30_000]) s = outageStep(s, false, at).state;
		const r = outageStep(s, false, 60_000);
		expect(r.event).toBe('down');
		s = r.state;
		expect(outageShown(s)).toBe(true);
		expect(outageStep(s, false, 70_000).event).toBeNull();
	});

	it('a success between failures resets the count', () => {
		let s = outageStep(outageStart(), false, 1000).state;
		s = outageStep(s, true, 2000).state;
		expect(outageStep(s, false, 3000).event).toBeNull();
	});

	it('needs both the count and the time', () => {
		const rules = { fails: 2, ms: 20_000 };
		const s = outageStep(outageStart(), false, 0, rules).state;
		expect(outageStep(s, false, 19_999, rules).event).toBeNull();
		expect(outageStep(s, false, 20_000, rules).event).toBe('down');
	});

	it('reports recovery once', () => {
		let s = goDown(0);
		const r = outageStep(s, true, 70_000);
		expect(r.event).toBe('up');
		s = r.state;
		expect(outageStep(s, true, 80_000).event).toBeNull();
	});

	it('a dismissal lasts for that outage only', () => {
		let s = goDown(0);
		s = outageDismiss(s);
		expect(outageShown(s)).toBe(false);
		s = outageStep(s, false, 70_000).state;
		expect(outageShown(s)).toBe(false);
		s = outageStep(s, true, 80_000).state;
		s = goDown(100_000, s);
		expect(outageShown(s)).toBe(true);
	});

	it('checks sooner while in doubt', () => {
		expect(nextCheckIn(outageStart())).toBe(60_000);
		expect(nextCheckIn(outageStep(outageStart(), false, 0).state)).toBe(10_000);
	});
});
