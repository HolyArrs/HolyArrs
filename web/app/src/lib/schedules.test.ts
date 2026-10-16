import { describe, expect, it } from 'vitest';
import { every, mayRun, stateTone, unit, until } from './schedules.ts';

describe('schedules', () => {
	it('says intervals in words', () => {
		expect(every(60)).toBe('every minute');
		expect(every(900)).toBe('every 15 minutes');
		expect(every(3600)).toBe('every hour');
		expect(every(43200)).toBe('every 12 hours');
		expect(every(86400)).toBe('every day');
		expect(every(90)).toBe('every 90 seconds');
		expect(every(0)).toBe('all the time');
	});
	it('picks the unit an interval is typed in', () => {
		expect(unit(3600)).toEqual({ size: 3600, word: 'hours' });
		expect(unit(60)).toEqual({ size: 60, word: 'minutes' });
		expect(unit(300)).toEqual({ size: 60, word: 'minutes' });
	});
	it('says when the next run is', () => {
		const now = Date.parse('2026-10-14T12:00:00Z');
		expect(until(undefined, now)).toBe('not planned');
		expect(until('2026-10-14T12:00:10Z', now)).toBe('now');
		expect(until('2026-10-14T12:15:00Z', now)).toBe('in 15 min');
		expect(until('2026-10-14T15:00:00Z', now)).toBe('in 3 h');
		expect(until('2026-10-16T12:00:00Z', now)).toBe('in 2 d');
	});
	it('colours states and offers Run now only when the part runs', () => {
		expect(stateTone('running')).toBe('wait');
		expect(stateTone('waiting')).toBe('ok');
		expect(stateTone('part_off')).toBe('');
		expect(mayRun({ state: 'waiting' })).toBe(true);
		expect(mayRun({ state: 'off' })).toBe(true);
		expect(mayRun({ state: 'part_off' })).toBe(false);
		expect(mayRun({ state: 'safe_mode' })).toBe(false);
	});
});
