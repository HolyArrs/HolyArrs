import { describe, expect, it } from 'vitest';
import { byDay, feedLink, monthGrid, monthRange, shiftMonth, webcalLink, ymd } from './calendar.ts';
import type { CalendarEntry } from './types.ts';

const e = (title_id: number, kind: CalendarEntry['kind'], date: string): CalendarEntry => ({ title_id, name: `Movie ${title_id}`, kind, date });

describe('calendar', () => {
	it('lays out whole weeks, Monday first', () => {
		// October 2026 starts on a Thursday and has 31 days.
		const r = monthRange(2026, 9);
		expect(r.from).toBe('2026-09-28');
		expect(r.to).toBe('2026-11-02');
		expect(r.weeks).toBe(5);
		const g = monthGrid(2026, 9, [e(1, 'cinema', '2026-10-13T00:00:00Z'), e(2, 'digital', '2026-10-13T00:00:00Z'), e(3, 'physical', '2026-11-01T00:00:00Z')], '2026-10-13');
		expect(g).toHaveLength(5);
		expect(g.every((w) => w.length === 7)).toBe(true);
		expect(g[0][0]).toMatchObject({ day: '2026-09-28', inMonth: false });
		const d13 = g.flat().find((d) => d.day === '2026-10-13')!;
		expect(d13.today).toBe(true);
		expect(d13.items.map((x) => x.kind)).toEqual(['cinema', 'digital']);
		expect(g[4][6]).toMatchObject({ day: '2026-11-01', inMonth: false });
		expect(g[4][6].items).toHaveLength(1);
	});

	it('groups the list by day', () => {
		const days = byDay([e(1, 'cinema', '2026-10-13T00:00:00Z'), e(2, 'digital', '2026-10-13T00:00:00Z'), e(3, 'physical', '2026-10-20T00:00:00Z')]);
		expect(days.map((d) => [d.day, d.items.length])).toEqual([
			['2026-10-13', 2],
			['2026-10-20', 1]
		]);
	});

	it('moves between months across years', () => {
		expect(shiftMonth(2026, 11, 1)).toEqual({ year: 2027, month: 0 });
		expect(shiftMonth(2026, 0, -1)).toEqual({ year: 2025, month: 11 });
		expect(ymd(new Date(2026, 0, 5))).toBe('2026-01-05');
	});

	it('makes the feed links', () => {
		const l = feedLink('http://192.0.2.10:8686/', '/api/v1/calendar/feed.ics?key=abc');
		expect(l).toBe('http://192.0.2.10:8686/api/v1/calendar/feed.ics?key=abc');
		expect(webcalLink(l)).toBe('webcal://192.0.2.10:8686/api/v1/calendar/feed.ics?key=abc');
	});
});
