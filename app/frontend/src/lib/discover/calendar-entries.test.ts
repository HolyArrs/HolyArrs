import { byDay, entryDay, entryHref, entryKey, shows } from './calendar.ts';
import { describe, expect, it } from 'vitest';
import type { CalendarEntry } from '../core/types.ts';

describe('episodes on the calendar', () => {
	const ep = (over: Partial<CalendarEntry>): CalendarEntry => ({ title_id: 3, name: 'Copper Valley', kind: 'episode', date: '2026-10-12T02:00:00Z', season: 1, episode: 2, item_id: 9, ...over });
	it('puts a timed episode on the local day and time', () => {
		const e = ep({ timed: true, air_day: '2026-10-11', episode_name: 'The Mill' });
		const local = new Date('2026-10-12T02:00:00Z');
		const p = (n: number) => String(n).padStart(2, '0');
		expect(entryDay(e)).toBe(`${local.getFullYear()}-${p(local.getMonth() + 1)}-${p(local.getDate())}`);
	});
	it('puts an episode without a time on its air day', () => {
		const e = ep({ timed: false, date: '2026-10-20T00:00:00Z', air_day: '2026-10-20' });
		expect(entryDay(e)).toBe('2026-10-20');
	});
	it('filters movies and shows and keeps days in order', () => {
		const m: CalendarEntry = { title_id: 1, name: 'Dune', kind: 'cinema', date: '2026-10-21T00:00:00Z' };
		const e = ep({ date: '2026-10-20T00:00:00Z', air_day: '2026-10-20' });
		expect(shows(m, 'movies') && !shows(e, 'movies')).toBe(true);
		expect(shows(e, 'shows') && !shows(m, 'shows')).toBe(true);
		expect(shows(m, 'all') && shows(e, 'all')).toBe(true);
		expect(byDay([m, e]).map((d) => d.day)).toEqual(['2026-10-20', '2026-10-21']);
	});
});

describe('music on the calendar', () => {
	const al: CalendarEntry = { title_id: 9, name: 'Lab Band', kind: 'album', date: '2026-10-20T00:00:00Z', part_id: 3, album: 'Old Days', year: 2026, tracks: 4, have: 1 };
	it('names, links and keys an album release', () => {
		expect(entryHref(al)).toBe('/library/9/albums/3');
		expect(entryHref({ ...al, kind: 'cinema' })).toBe('/library/9');
		expect(entryKey(al)).toBe('a3');
	});
	it('filters by movies, shows or music', () => {
		expect(shows(al, 'music')).toBe(true);
		expect(shows(al, 'movies')).toBe(false);
		expect(shows(al, 'shows')).toBe(false);
		expect(shows(al, 'all')).toBe(true);
		expect(shows({ kind: 'cinema' }, 'movies')).toBe(true);
		expect(shows({ kind: 'episode' }, 'music')).toBe(false);
	});
});
