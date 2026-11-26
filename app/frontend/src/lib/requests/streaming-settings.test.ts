import { describe, expect, it } from 'vitest';
import { MAX_MY_SERVICES, countryOptions, toggleService } from './streaming-settings.ts';

describe('toggleService', () => {
	const full = Array.from({ length: MAX_MY_SERVICES }, (_, i) => i + 1);
	it.each([
		{ name: 'adds at the end', picked: [8], id: 337, on: true, want: [8, 337] },
		{ name: 'removes', picked: [8, 337], id: 8, on: false, want: [337] },
		{ name: 'adding twice keeps one', picked: [8], id: 8, on: true, want: [8] },
		{ name: 'removing a missing one', picked: [8], id: 9, on: false, want: [8] },
		{ name: 'stops at the limit', picked: full, id: 999, on: true, want: full },
		{ name: 'from empty', picked: [], id: 2, on: true, want: [2] }
	])('$name', ({ picked, id, on, want }) => {
		expect(toggleService(picked, id, on)).toEqual(want);
	});
});

describe('countryOptions', () => {
	it.each([
		{ name: 'listed country is there once', saved: 'US', has: 'US', count: 1 },
		{ name: 'unlisted saved country is kept', saved: 'LU', has: 'LU', count: 1 },
		{ name: 'odd saved value is left out', saved: 'usa', has: 'usa', count: 0 }
	])('$name', ({ saved, has, count }) => {
		expect(countryOptions(saved, 'en').filter((c) => c.code === has)).toHaveLength(count);
	});
	it('sorts by shown name', () => {
		const names = countryOptions('', 'en').map((c) => c.name);
		expect(names).toEqual([...names].sort((a, b) => a.localeCompare(b, 'en')));
	});
});
