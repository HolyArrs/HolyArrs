import { describe, expect, it } from 'vitest';
import { bringsItems, hasCount } from './brings-counts.ts';

describe('bringsItems', () => {
	const counts = { titles: 1252, folders: 1, quality_profiles: 6, custom_formats: 14, notifications: 0 };
	it('pairs each number with its word, in the locale', () => {
		const [m] = bringsItems('migrate.brings.movies', 'radarr', counts, 'en');
		expect(m.num).toBe('1,252');
		expect(m.text).toContain('1,252');
		expect(m.text).not.toBe('1,252');
		expect(bringsItems('migrate.brings.quality', 'radarr', counts, 'en')).toHaveLength(2);
	});
	it('marks zero and skips lines without a count', () => {
		expect(bringsItems('migrate.brings.alerts', 'radarr', counts, 'en')[0].zero).toBe(true);
		expect(bringsItems('migrate.brings.naming', 'radarr', counts, 'en')).toEqual([]);
		expect(bringsItems('migrate.brings.movies', 'radarr', null, 'en')).toEqual([]);
		expect(hasCount('migrate.brings.naming')).toBe(false);
	});
});
