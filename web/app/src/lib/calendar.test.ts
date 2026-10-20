import { describe, expect, it } from 'vitest';
import { itemDot, passes, readPresets, savePresets, stacked, stackedRange, weekLabel, byDay, feedLink, monthGrid, monthRange, shiftMonth, webcalLink, ymd, weekStart, weekRange, weekDays, shiftWeek, readView, saveView, startView, entryArt, entryIcon } from './calendar.ts';
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

describe('calendar week', () => {
	it('starts weeks on Monday and lays out seven days', () => {
		const s = weekStart(new Date(2026, 9, 18)); // a Sunday
		expect(ymd(s)).toBe('2026-10-12');
		expect(weekRange(s)).toEqual({ from: '2026-10-12', to: '2026-10-19' });
		const days = weekDays(s, [e(1, 'cinema', '2026-10-13T00:00:00Z'), e(2, 'cinema', '2026-10-20T00:00:00Z')], '2026-10-18');
		expect(days.map((d) => d.date)).toEqual([12, 13, 14, 15, 16, 17, 18]);
		expect(days[1].items).toHaveLength(1);
		expect(days[6].today).toBe(true);
		expect(ymd(shiftWeek(s, -1))).toBe('2026-10-05');
	});

	it('opens on week for shows on wide screens, and keeps the last choice', () => {
		expect(startView({ remembered: null, which: 'shows', wide: true, monthLink: false })).toBe('week');
		expect(startView({ remembered: null, which: 'all', wide: true, monthLink: false })).toBe('week');
		expect(startView({ remembered: null, which: 'movies', wide: true, monthLink: false })).toBe('month');
		expect(startView({ remembered: null, which: 'shows', wide: false, monthLink: false })).toBe('list');
		expect(startView({ remembered: 'month', which: 'shows', wide: true, monthLink: false })).toBe('month');
		expect(startView({ remembered: 'week', which: 'shows', wide: true, monthLink: true })).toBe('month');
		expect(startView({ remembered: 'week', which: 'shows', wide: false, monthLink: true })).toBe('list');
		expect(startView({ remembered: 'month', which: 'shows', wide: false, monthLink: true })).toBe('month');
	});

	it('remembers the view, and survives storage that throws', () => {
		const m = new Map<string, string>();
		const store = { getItem: (k: string) => m.get(k) ?? null, setItem: (k: string, v: string) => void m.set(k, v) };
		expect(readView(store)).toBeNull();
		saveView('week', store);
		expect(readView(store)).toBe('week');
		const broken = { getItem: () => { throw new Error('no'); }, setItem: () => { throw new Error('no'); } };
		expect(readView(broken)).toBeNull();
		expect(() => saveView('list', broken)).not.toThrow();
	});
});

describe('calendar artwork', () => {
	it('uses the still, then the backdrop, then the poster', () => {
		expect(entryArt({ ...e(1, 'episode', '2026-10-20'), still: 's.jpg', backdrop: 'b.jpg', poster: 'p.jpg' })).toBe('s.jpg');
		expect(entryArt({ ...e(1, 'episode', '2026-10-20'), backdrop: 'b.jpg', poster: 'p.jpg' })).toBe('b.jpg');
		expect(entryArt({ ...e(1, 'digital', '2026-10-20'), still: '', poster: 'p.jpg' })).toBe('p.jpg');
		expect(entryArt(e(1, 'digital', '2026-10-20'))).toBeNull();
	});
	it('picks the kind icon for entries without art', () => {
		expect(entryIcon('episode')).toBe('tv');
		expect(entryIcon('album')).toBe('music');
		expect(entryIcon('cinema')).toBe('film');
	});
});

describe('stacked weeks', () => {
	const ep = (date: string, state?: string): CalendarEntry => ({ title_id: 1, name: 'X', kind: 'episode', date, air_day: date, timed: false, state });
	it('names the weeks', () => {
		const today = new Date(2026, 9, 21);
		expect(weekLabel(weekStart(today), today)).toBe('This week');
		expect(weekLabel(shiftWeek(weekStart(today), 1), today)).toBe('Next week');
		expect(weekLabel(shiftWeek(weekStart(today), 3), today)).not.toMatch(/week/);
	});
	it('lays out two weeks and Later', () => {
		const start = new Date(2026, 9, 19);
		const s = stacked(start, [ep('2026-10-20'), ep('2026-10-28'), ep('2026-11-05'), ep('2026-12-30')], '2026-10-21');
		expect(s.weeks).toHaveLength(2);
		expect(s.weeks[0].days[1].items).toHaveLength(1);
		expect(s.weeks[0].days[2].today).toBe(true);
		expect(s.later.map((d) => d.day)).toEqual(['2026-11-05']);
		expect(stackedRange(start)).toEqual({ from: '2026-10-19', to: '2026-11-30' });
	});
	it('picks a status dot and filters', () => {
		expect(itemDot(ep('2026-10-20', 'in_library'), '2026-10-21')).toBe('have');
		expect(itemDot(ep('2026-10-20'), '2026-10-21')).toBe('missing');
		expect(itemDot(ep('2026-10-22'), '2026-10-21')).toBe('upcoming');
		expect(passes({ ...ep('2026-10-22'), keep_up_to_date: false }, { keptOnly: true, haveOnly: false })).toBe(false);
		expect(passes(ep('2026-10-22'), { keptOnly: false, haveOnly: true })).toBe(false);
	});
	it('saves and reads filter presets', () => {
		const m = new Map<string, string>();
		const store = { getItem: (k: string) => m.get(k) ?? null, setItem: (k: string, v: string) => void m.set(k, v) };
		expect(readPresets(store)).toEqual([]);
		const p = { name: 'Shows I keep', which: 'shows' as const, off: [], keptOnly: true, haveOnly: false };
		savePresets([p], store);
		expect(readPresets(store)).toEqual([p]);
		m.set('holyarrs.calendar-filters', '{bad');
		expect(readPresets(store)).toEqual([]);
	});
});
