import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { airWords, dayWords, epTone } from './title-library-words.ts';

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

describe('episode row tone with a file', () => {
	it('shows a file the episode holds as have, whatever the state says', () => {
		expect(epTone('not_out_yet', true)).toBe('have');
		expect(epTone('better_wanted', true)).toBe('have');
		expect(epTone('wanted', true)).toBe('have');
		expect(epTone('not_out_yet')).toBe('missing');
		expect(epTone('searching', true)).toBe('downloading');
	});
});

describe('airWords: the date and "not out yet" come from the same moment', () => {
	const zone = process.env.TZ;
	afterEach(() => {
		vi.useRealTimers();
		if (zone === undefined) delete process.env.TZ; else process.env.TZ = zone;
	});

	it('a show that airs tonight in the US is not "Yesterday" for a viewer in Sydney', () => {
		process.env.TZ = 'Australia/Sydney';
		vi.useFakeTimers();
		// Fri Nov 13, 10:53 in Sydney; the US show's day was Thu Nov 12 and it airs Nov 13 02:00 UTC.
		vi.setSystemTime(new Date('2026-11-12T23:53:00Z'));
		expect(dayWords('2026-11-12')).toBe('Yesterday');
		expect(airWords('2026-11-12', '2026-11-13T02:00:00Z')).toBe('Today');
		expect(airWords('2026-11-12', '2026-11-13T00:00:00Z')).toBe('Today');
	});

	it('keeps the own day once the moment has passed, and for future days', () => {
		process.env.TZ = 'Europe/Berlin';
		vi.useFakeTimers();
		vi.setSystemTime(new Date('2026-11-13T09:53:00Z'));
		expect(airWords('2026-11-12', '2026-11-13T02:00:00Z')).toBe('Yesterday');
		expect(airWords('2026-11-20', '2026-11-21T02:00:00Z')).toBe(dayWords('2026-11-20'));
		expect(airWords(undefined, undefined)).toBe('');
	});
});
