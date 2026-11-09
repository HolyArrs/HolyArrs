import { describe, expect, it } from 'vitest';
import { buildingStatus, holdFor, loadStage, MIN_SHOW_MS, SHOW_AFTER_MS, stageTimers } from './silhouette.ts';

describe('silhouette timing', () => {
	it('stays quiet for the first 150 ms', () => {
		expect(loadStage(0)).toBe('quiet');
		expect(loadStage(149)).toBe('quiet');
		expect(loadStage(150)).toBe('skeleton');
	});
	it('adds the Orbit at 500 ms and reassurance at 10 s', () => {
		expect(loadStage(499)).toBe('skeleton');
		expect(loadStage(500)).toBe('orbit');
		expect(loadStage(9_999)).toBe('orbit');
		expect(loadStage(10_000)).toBe('reassure');
	});
	it('swaps fast data in at once', () => {
		expect(holdFor(0)).toBe(0);
		expect(holdFor(149)).toBe(0);
	});
	it('keeps a shown skeleton for about 100 ms', () => {
		expect(MIN_SHOW_MS).toBe(100);
		expect(holdFor(150)).toBe(MIN_SHOW_MS);
		expect(holdFor(200)).toBe(SHOW_AFTER_MS + MIN_SHOW_MS - 200);
		expect(holdFor(250)).toBe(0);
		expect(holdFor(550)).toBe(0);
		expect(holdFor(4000)).toBe(0);
	});
	it('swaps an answer from the kept answers in at once', () => {
		expect(holdFor(150, true)).toBe(0);
		expect(holdFor(200, true)).toBe(0);
	});
	it('lists only the stages still ahead', () => {
		expect(stageTimers(0).map((t) => t.stage)).toEqual(['skeleton', 'orbit', 'reassure']);
		expect(stageTimers(600)).toEqual([{ at: 9_400, stage: 'reassure' }]);
		expect(stageTimers(20_000)).toEqual([]);
	});
	it('counts building tries', () => {
		expect(buildingStatus(1)).toBe('Building this page…');
		expect(buildingStatus(3, 'New & Popular')).toBe('Building New & Popular… try 3');
	});
});
