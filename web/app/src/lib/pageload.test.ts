import { describe, expect, it } from 'vitest';
import { pageHoldFor, pageLine, pageStage, pageTimers } from './pageload.ts';

describe('page loading timing', () => {
	it('shows nothing for the first 300 ms', () => {
		expect(pageStage(0)).toBe('hidden');
		expect(pageStage(299)).toBe('hidden');
		expect(pageStage(300)).toBe('orbit');
	});
	it('adds a status at 3 s and reassurance at 10 s', () => {
		expect(pageStage(2_999)).toBe('orbit');
		expect(pageStage(3_000)).toBe('status');
		expect(pageStage(9_999)).toBe('status');
		expect(pageStage(10_000)).toBe('reassure');
	});
	it('lets fast pages appear at once', () => {
		expect(pageHoldFor(0)).toBe(0);
		expect(pageHoldFor(299)).toBe(0);
	});
	it('keeps a shown Orbit at least 500 ms', () => {
		expect(pageHoldFor(300)).toBe(500);
		expect(pageHoldFor(600)).toBe(200);
		expect(pageHoldFor(800)).toBe(0);
		expect(pageHoldFor(5_000)).toBe(0);
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
