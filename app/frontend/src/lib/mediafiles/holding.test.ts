import { describe, expect, it } from 'vitest';
import { daysLeft, daysLeftWords, fileName } from './holding.ts';

describe('holding', () => {
	const now = Date.parse('2026-11-03T12:00:00Z');
	it('counts whole days left, rounding up', () => {
		expect(daysLeft('2026-11-05T00:00:00Z', now)).toBe(2);
		expect(daysLeft('2026-11-04T00:00:00Z', now)).toBe(1);
		expect(daysLeft('2026-11-01T00:00:00Z', now)).toBe(0);
		expect(daysLeft(undefined, now)).toBeNull();
		expect(daysLeft('soon', now)).toBeNull();
	});
	it('says it in plain words', () => {
		expect(daysLeftWords('2026-11-10T00:00:00Z', now)).toBe('7 days left');
		expect(daysLeftWords('2026-11-04T00:00:00Z', now)).toBe('1 day left');
		expect(daysLeftWords('2026-11-03T00:00:00Z', now)).toBe('Goes today');
		expect(daysLeftWords(undefined, now)).toBe('Kept until you empty it');
	});
	it('takes the file name off a path', () => {
		expect(fileName('Dune (2024)/Dune.mkv')).toBe('Dune.mkv');
		expect(fileName('Dune.mkv')).toBe('Dune.mkv');
	});
});
