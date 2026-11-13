import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { dayWords } from './title-library-words.ts';

describe('dayWords', () => {
	const zone = process.env.TZ;
	beforeEach(() => {
		process.env.TZ = 'America/New_York';
		vi.useFakeTimers();
		vi.setSystemTime(new Date('2026-11-10T15:00:00Z'));
	});
	afterEach(() => {
		vi.useRealTimers();
		if (zone === undefined) delete process.env.TZ; else process.env.TZ = zone;
	});

	it('shows an evening air time on the viewer\'s own day, like the episode row', () => {
		// 8pm in New York on Sun Nov 15 is already Mon Nov 16 in UTC.
		expect(dayWords('2026-11-16T01:00:00Z')).toBe(dayWords('2026-11-15'));
		expect(dayWords('2026-11-16T01:00:00Z')).toContain('15');
	});

	it('keeps a bare day on that day', () => {
		expect(dayWords('2026-11-15')).toContain('15');
		expect(dayWords('2026-11-15T00:00:00Z')).toContain('15');
	});
});
