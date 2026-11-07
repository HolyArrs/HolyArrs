import { describe, expect, it } from 'vitest';
import { albumHave, byDay, entryDay, entryHref, entryKey, entryName, entryTime, shows } from './calendar.ts';
import type { CalendarEntry } from '../core/types.ts';

describe('episodes on the calendar', () => {
	const ep = (over: Partial<CalendarEntry>): CalendarEntry => ({ title_id: 3, name: 'Copper Valley', kind: 'episode', date: '2026-10-12T02:00:00Z', season: 1, episode: 2, item_id: 9, ...over });
	it('puts a timed episode on the local day and time', () => {
		const e = ep({ timed: true, air_day: '2026-10-11', episode_name: 'The Mill' });
		const local = new Date('2026-10-12T02:00:00Z');
		const p = (n: number) => String(n).padStart(2, '0');
		expect(entryDay(e)).toBe(`${local.getFullYear()}-${p(local.getMonth() + 1)}-${p(local.getDate())}`);
		expect(entryTime(e)).toBe(`${p(local.getHours())}:${p(local.getMinutes())}`);
		expect(entryName(e)).toBe('Copper Valley S01E02 The Mill');
	});
	it('uses the viewer time zone, not the UTC date or the air day', () => {
		const was = process.env.TZ;
		process.env.TZ = 'America/New_York';
		try {
			const e = ep({ timed: true, date: '2026-10-12T02:00:00Z', air_day: '2026-10-12' });
			expect(entryDay(e)).toBe('2026-10-11');
			expect(entryTime(e)).toBe('22:00');
			process.env.TZ = 'Asia/Tokyo';
			expect(entryDay(ep({ timed: true, date: '2026-10-12T20:00:00Z', air_day: '2026-10-12' }))).toBe('2026-10-13');
		} finally {
			if (was === undefined) delete process.env.TZ;
			else process.env.TZ = was;
		}
	});
	it('puts an episode without a time on its air day', () => {
		const e = ep({ timed: false, date: '2026-10-20T00:00:00Z', air_day: '2026-10-20' });
		expect(entryDay(e)).toBe('2026-10-20');
		expect(entryTime(e)).toBe('');
		expect(entryName({ title_id: 1, name: 'Dune', year: 2024, kind: 'digital', date: '2024-04-16T00:00:00Z' })).toBe('Dune (2024)');
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
		expect(entryName(al)).toBe('Lab Band – Old Days');
		expect(entryHref(al)).toBe('/library/9/albums/3');
		expect(entryHref({ ...al, kind: 'cinema' })).toBe('/library/9');
		expect(entryKey(al)).toBe('a3');
		expect(albumHave(al)).toBe('1 of 4 songs in your Library');
		expect(albumHave({ tracks: 4, have: 4 })).toBe('In your Library');
		expect(albumHave({ tracks: 4, have: 0 })).toBe('Not in your Library yet');
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
