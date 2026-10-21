import { describe, expect, it } from 'vitest';
import { nextCheckIn, outageDismiss, outageShown, outageStart, outageStep } from './outage.ts';

describe('outage', () => {
	it('ignores one failed check', () => {
		const r = outageStep(outageStart(), false, 1000);
		expect(r.event).toBeNull();
		expect(outageShown(r.state)).toBe(false);
	});

	it('goes down after two failures in a row', () => {
		let s = outageStep(outageStart(), false, 1000).state;
		const r = outageStep(s, false, 2000);
		expect(r.event).toBe('down');
		s = r.state;
		expect(outageShown(s)).toBe(true);
		expect(outageStep(s, false, 3000).event).toBeNull();
	});

	it('a success between failures resets the count', () => {
		let s = outageStep(outageStart(), false, 1000).state;
		s = outageStep(s, true, 2000).state;
		expect(outageStep(s, false, 3000).event).toBeNull();
	});

	it('goes down after 20 s of failure even with a looser count', () => {
		const rules = { fails: 99, ms: 20_000 };
		const s = outageStep(outageStart(), false, 0, rules).state;
		expect(outageStep(s, false, 19_999, rules).event).toBeNull();
		expect(outageStep(s, false, 20_000, rules).event).toBe('down');
	});

	it('reports recovery once', () => {
		let s = outageStep(outageStep(outageStart(), false, 0).state, false, 1).state;
		const r = outageStep(s, true, 2);
		expect(r.event).toBe('up');
		s = r.state;
		expect(outageStep(s, true, 3).event).toBeNull();
	});

	it('a dismissal lasts for that outage only', () => {
		let s = outageStep(outageStep(outageStart(), false, 0).state, false, 1).state;
		s = outageDismiss(s);
		expect(outageShown(s)).toBe(false);
		s = outageStep(s, false, 2).state;
		expect(outageShown(s)).toBe(false);
		s = outageStep(s, true, 3).state;
		s = outageStep(outageStep(s, false, 4).state, false, 5).state;
		expect(outageShown(s)).toBe(true);
	});

	it('checks sooner while in doubt', () => {
		expect(nextCheckIn(outageStart())).toBe(60_000);
		expect(nextCheckIn(outageStep(outageStart(), false, 0).state)).toBe(10_000);
	});
});
