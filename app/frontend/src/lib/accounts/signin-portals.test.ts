import { LEAD_S, legAt, holeAt, symbolAt, LEG_GAP_S, START_S, COUNT, TRAVEL_S, SPACING_S } from './signin-portals.ts';
import { describe, expect, it } from 'vitest';

describe('signin portals timeline', () => {
	it('starts 9 s in and loops the four legs with long rests', () => {
		expect(legAt(START_S - 0.1, 4)).toBeNull();
		expect(legAt(START_S, 4)).toEqual({ index: 0, local: 0 });
		expect(legAt(START_S + 3 * LEG_GAP_S + 1, 4)?.index).toBe(3);
		expect(legAt(START_S + 4 * LEG_GAP_S, 4)?.index).toBe(0);
	});
	it('holes pop open and are shut outside their window', () => {
		expect(holeAt(0, 'from').s).toBe(0);
		expect(holeAt(0.3, 'from').s).toBeGreaterThan(0.5);
		expect(holeAt(0.3, 'from').s).toBeLessThanOrEqual(1); // Soft, no overshoot.
		expect(holeAt(0.8, 'from').s).toBe(1);
		expect(holeAt(0.8, 'to').s).toBe(0);
	});
	it('the far hole is open while symbols arrive', () => {
		for (let i = 0; i < COUNT; i++) {
			const arrive = LEAD_S + i * SPACING_S + TRAVEL_S - 0.01;
			expect(symbolAt(arrive, i)).not.toBeNull();
			expect(holeAt(arrive, 'to').s).toBeGreaterThan(0.9);
		}
	});
});
