import { describe, expect, it } from 'vitest';
import { weekRange } from './calendar.ts';

// ICU puts thin or narrow spaces around the dash; compare on plain spaces.
const plain = (s: string) => s.replace(/[\u2009\u202f\u00a0]/g, ' ');

describe('weekRange', () => {
	const now = new Date(2026, 10, 11);
	it('names the month once inside one month and drops this year', () => {
		expect(plain(weekRange(new Date(2026, 10, 9), new Date(2026, 10, 15), 'en-US', now))).toBe('Nov 9 – 15');
	});
	it('keeps both months across a month end', () => {
		expect(plain(weekRange(new Date(2026, 10, 30), new Date(2026, 11, 6), 'en-US', now))).toBe('Nov 30 – Dec 6');
	});
	it('adds the year once for another year', () => {
		expect(plain(weekRange(new Date(2025, 10, 24), new Date(2025, 10, 30), 'en-US', now))).toBe('Nov 24 – 30, 2025');
		expect(plain(weekRange(new Date(2026, 11, 28), new Date(2027, 0, 3), 'en-US', now))).toBe('Dec 28, 2026 – Jan 3, 2027');
	});
	it('follows the language order', () => {
		expect(plain(weekRange(new Date(2025, 10, 24), new Date(2025, 10, 30), 'de', now))).toBe('24.–30. Nov. 2025');
	});
	it('still gives both dates when the locale is not known', () => {
		expect(weekRange(new Date(2026, 10, 9), new Date(2026, 10, 15), 'not a locale', now)).toMatch(/9.*15/);
	});
});
