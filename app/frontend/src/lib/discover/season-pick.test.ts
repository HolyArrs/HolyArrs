import { describe, expect, it } from 'vitest';
import { deriveSeasonPick } from './season-pick.ts';

describe('deriveSeasonPick', () => {
	it('preserves latest vs all on a single-season show', () => {
		const one = [{ number: 1, keep_up_to_date: true }];
		expect(deriveSeasonPick(one, 'all', 'latest')).toBe('latest');
		expect(deriveSeasonPick(one, 'all', 'all')).toBe('all');
		expect(deriveSeasonPick(one, 'all', undefined)).toBe('all');
	});

	it('returns none or custom on a single-season show when season 1 is off', () => {
		const off = [{ number: 1, keep_up_to_date: false }];
		expect(deriveSeasonPick(off, 'none', 'none')).toBe('none');
		expect(deriveSeasonPick(off, 'all', 'latest')).toBe('custom');
	});

	it('handles multi-season shows', () => {
		expect(
			deriveSeasonPick(
				[
					{ number: 1, keep_up_to_date: true },
					{ number: 2, keep_up_to_date: true }
				],
				'all',
				'all'
			)
		).toBe('all');

		expect(
			deriveSeasonPick(
				[
					{ number: 1, keep_up_to_date: false },
					{ number: 2, keep_up_to_date: true }
				],
				'all',
				'latest'
			)
		).toBe('latest');

		expect(
			deriveSeasonPick(
				[
					{ number: 1, keep_up_to_date: true },
					{ number: 2, keep_up_to_date: false }
				],
				'all',
				'all'
			)
		).toBe('custom');

		expect(
			deriveSeasonPick(
				[
					{ number: 1, keep_up_to_date: false },
					{ number: 2, keep_up_to_date: false }
				],
				'none',
				'none'
			)
		).toBe('none');
	});

	it('ignores specials (season 0) when deriving the pick', () => {
		const withSpecials = [
			{ number: 0, keep_up_to_date: false },
			{ number: 1, keep_up_to_date: true }
		];
		expect(deriveSeasonPick(withSpecials, 'all', 'latest')).toBe('latest');
		expect(deriveSeasonPick(withSpecials, 'all', 'all')).toBe('all');
	});
});
