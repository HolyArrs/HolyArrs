import { describe, expect, it } from 'vitest';
import { filePointsWhy, type FilePointsView } from './file-points.ts';

const base: FilePointsView = { step: '1080p_web', good_enough_at: '1080p_web', step_reached: true, points: 1675, good_enough_points: 1500, good_enough: true, meets_version: true, traits: [], names: {} };
const step = (s: string) => s.toUpperCase();

describe('filePointsWhy', () => {
	it('says nothing for a file that is good enough', () => {
		expect(filePointsWhy(base, step)).toEqual([]);
	});
	it('names the step, the points short and the version', () => {
		const why = filePointsWhy({ ...base, step: '720p_web', step_reached: false, points: 75, good_enough_points: 1600, good_enough: false, meets_version: false }, step);
		expect(why).toHaveLength(3);
		expect(why[0]).toContain('720P_WEB');
		expect(why[0]).toContain('1080P_WEB');
		expect(why[1]).toContain('1525');
	});
	it('reads an unknown step and no points bar', () => {
		const why = filePointsWhy({ ...base, step: '', step_reached: false, good_enough_points: null, good_enough: false }, step);
		expect(why).toHaveLength(1);
	});
});
