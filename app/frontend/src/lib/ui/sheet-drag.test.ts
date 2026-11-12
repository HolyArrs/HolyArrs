import { describe, expect, it } from 'vitest';
import { dragShuts, dragOffset } from './sheet-drag.ts';

describe('sheet drag', () => {
	it('shuts past 30% of the sheet height', () => {
		expect(dragShuts(150, 400, 2000)).toBe(true);
		expect(dragShuts(100, 400, 2000)).toBe(false);
	});
	it('shuts on a quick flick even when short', () => {
		expect(dragShuts(40, 400, 200)).toBe(true);
		expect(dragShuts(40, 400, 800)).toBe(false);
	});
	it('never shuts on an upward or zero drag', () => {
		expect(dragShuts(0, 400, 10)).toBe(false);
		expect(dragShuts(-80, 400, 50)).toBe(false);
	});
	it('follows the finger down and resists going up', () => {
		expect(dragOffset(120)).toBe(120);
		expect(dragOffset(-100)).toBeLessThan(0);
		expect(dragOffset(-100)).toBeGreaterThan(-20);
	});
});
