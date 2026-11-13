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

describe('season header next day', () => {
	const zone = process.env.TZ;
	afterEach(() => {
		vi.useRealTimers();
		if (zone === undefined) delete process.env.TZ; else process.env.TZ = zone;
	});

	it.each(['Europe/Berlin', 'Asia/Tokyo'])('matches the episode row in %s', (tz) => {
		process.env.TZ = tz;
		vi.useFakeTimers();
		vi.setSystemTime(new Date('2026-11-10T15:00:00Z'));
		const season = { next_air_date: '2026-11-16T01:30:00Z', next_air_day: '2026-11-15' };
		const header = dayWords(season.next_air_day || season.next_air_date);
		const row = dayWords('2026-11-15');
		expect(header).toBe(row);
		expect(header).toContain('15');
		expect(dayWords(season.next_air_date)).not.toBe(row);
	});
});
