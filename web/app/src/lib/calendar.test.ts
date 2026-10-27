import { describe, expect, it } from 'vitest';
import { addDays, rangeChunks, titlesFromAddress, itemDot, passes, readPresets, savePresets, byDay, feedLink, monthGrid, monthRange, shiftMonth, webcalLink, ymd, weekStart, shiftWeek, readView, saveView, startView, viewFromAddress, entryArt, entryIcon } from './calendar.ts';
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
	it('starts weeks on Monday', () => {
		const s = weekStart(new Date(2026, 9, 18)); // a Sunday
		expect(ymd(s)).toBe('2026-10-12');
		expect(ymd(shiftWeek(s, -1))).toBe('2026-10-05');
	});

	it('opens on Cards, the address first, then the session pick; old layout links still land', () => {
		expect(startView(null, null)).toBe('cards');
		expect(startView(null, 'list')).toBe('list');
		expect(startView('cards', 'list')).toBe('cards');
		expect(viewFromAddress('2')).toBe('cards');
		expect(viewFromAddress('5')).toBe('cards');
		expect(viewFromAddress('now')).toBe('list');
		expect(viewFromAddress('week')).toBeNull();
	});

	it('remembers the view, and survives storage that throws', () => {
		const m = new Map<string, string>();
		const store = { getItem: (k: string) => m.get(k) ?? null, setItem: (k: string, v: string) => void m.set(k, v) };
		expect(readView(store)).toBeNull();
		saveView('list', store);
		expect(readView(store)).toBe('list');
		const broken = { getItem: () => { throw new Error('no'); }, setItem: () => { throw new Error('no'); } };
		expect(readView(broken)).toBeNull();
		expect(() => saveView('cards', broken)).not.toThrow();
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

describe('status and filters', () => {
	const ep = (date: string, state?: string): CalendarEntry => ({ title_id: 1, name: 'X', kind: 'episode', date, air_day: date, timed: false, state });
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

describe('calendar range', () => {
	it('moves by days across months', () => {
		expect(ymd(addDays(new Date(2026, 9, 27), -30))).toBe('2026-09-27');
		expect(ymd(addDays(new Date(2026, 11, 31), 1))).toBe('2027-01-01');
	});
	it('splits a long query into pieces the server takes', () => {
		expect(rangeChunks('2026-10-01', '2026-10-08')).toEqual([{ from: '2026-10-01', to: '2026-10-08' }]);
		const c = rangeChunks('2025-10-27', '2026-12-08');
		expect(c).toHaveLength(2);
		expect(c[0].from).toBe('2025-10-27');
		expect(c[0].to).toBe(c[1].from);
		expect(c[1].to).toBe('2026-12-08');
		expect(rangeChunks('2026-10-01', '2026-10-01')).toEqual([]);
	});
	it('reads the titles in the address, old and new words', () => {
		expect(titlesFromAddress('shows')).toBe('shows');
		expect(titlesFromAddress('all')).toBe('all');
		expect(titlesFromAddress('everything')).toBe('all');
		expect(titlesFromAddress(null)).toBe('all');
	});
});
