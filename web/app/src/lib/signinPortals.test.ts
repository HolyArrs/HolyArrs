import { describe, expect, it } from 'vitest';
import { legAt, holeAt, symbolAt, LEG_S, LEG_GAP_S, START_S, COUNT, TRAVEL_S, SPACING_S } from './signinPortals.ts';

describe('signin portals timeline', () => {
	it('starts 5 s in and loops the four legs 5 s apart', () => {
		expect(legAt(4.9, 4)).toBeNull();
		expect(legAt(START_S, 4)).toEqual({ index: 0, local: 0 });
		expect(legAt(START_S + 3 * LEG_GAP_S + 1, 4)?.index).toBe(3);
		expect(legAt(START_S + 4 * LEG_GAP_S, 4)?.index).toBe(0);
	});
	it('a leg finishes before the next one starts', () => {
		expect(LEG_S).toBeLessThan(LEG_GAP_S);
		expect(holeAt(LEG_S, 'to').s).toBe(0);
		expect(holeAt(LEG_S, 'from').s).toBe(0);
	});
	it('holes pop open and are shut outside their window', () => {
		expect(holeAt(0, 'from').s).toBe(0);
		expect(holeAt(0.3, 'from').s).toBeGreaterThan(1); // overshoot
		expect(holeAt(0.6, 'from').s).toBe(1);
		expect(holeAt(0.6, 'to').s).toBe(0);
	});
	it('the far hole is open while symbols arrive', () => {
		for (let i = 0; i < COUNT; i++) {
			const arrive = 0.18 + i * SPACING_S + TRAVEL_S - 0.01;
			expect(symbolAt(arrive, i)).not.toBeNull();
			expect(holeAt(arrive, 'to').s).toBeGreaterThan(0.9);
		}
	});
});
