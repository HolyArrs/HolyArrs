import { describe, expect, it } from 'vitest';
import { pageHoldFor, pageLine, pageTimers } from './pageload.ts';

describe('page loading timing', () => {
	it('lets fast pages appear at once', () => {
		expect(pageHoldFor(0)).toBe(0);
		expect(pageHoldFor(299)).toBe(0);
	});
	it('keeps a shown Orbit about 100 ms', () => {
		expect(pageHoldFor(300)).toBe(100);
		expect(pageHoldFor(350)).toBe(50);
		expect(pageHoldFor(400)).toBe(0);
		expect(pageHoldFor(800)).toBe(0);
		expect(pageHoldFor(5_000)).toBe(0);
	});
	it('swaps a kept answer in at once', () => {
		expect(pageHoldFor(320, true)).toBe(0);
	});
	it('schedules only the stages still ahead', () => {
		expect(pageTimers(0)).toEqual([
			{ at: 300, stage: 'orbit' },
			{ at: 3_000, stage: 'status' },
			{ at: 10_000, stage: 'reassure' }
		]);
		expect(pageTimers(3_500)).toEqual([{ at: 6_500, stage: 'reassure' }]);
	});
	it('says the page text, or a default, from 3 s', () => {
		expect(pageLine('orbit', 'Loading Explore…')).toBe('');
		expect(pageLine('status')).toBe('Getting things ready…');
		expect(pageLine('status', 'Building Explore… try 2')).toBe('Building Explore… try 2');
	});
	it('shows a building attempt as soon as the Orbit shows', () => {
		expect(pageLine('hidden', 'Building Explore… try 2', true)).toBe('');
		expect(pageLine('orbit', 'Building Explore… try 2', true)).toBe('Building Explore… try 2');
	});
});
