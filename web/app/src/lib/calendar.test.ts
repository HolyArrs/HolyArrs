import { describe, expect, it } from 'vitest';
import { addDays, rangeChunks, titlesFromAddress, itemDot, oneMovieDate, movieFacts, mineOf, whoseOk, scopeOk, statusOk, showingOk, filtersFromAddress, type MovieFacts, byDay, feedLink, monthGrid, monthRange, shiftMonth, webcalLink, ymd, weekStart, shiftWeek, readView, saveView, startView, viewFromAddress, entryArt, entryIcon } from './calendar.ts';
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
	});
});

describe('one date per movie', () => {
	const mv = (id: number, kind: CalendarEntry['kind'], day: string): CalendarEntry => ({ title_id: id, name: `Movie ${id}`, kind, date: `${day}T00:00:00Z` });
	it('keeps the home date (the earlier of digital and disc), else the cinema date', () => {
		const out = oneMovieDate([mv(1, 'cinema', '2026-11-01'), mv(1, 'digital', '2026-12-01'), mv(1, 'physical', '2026-12-20'), mv(2, 'cinema', '2026-11-05')]);
		expect(out.map((x) => [x.title_id, x.kind])).toEqual([
			[1, 'digital'],
			[2, 'cinema']
		]);
		// Digital and disc on one day: one date.
		expect(oneMovieDate([mv(3, 'digital', '2026-12-01'), mv(3, 'physical', '2026-12-01')])).toHaveLength(1);
	});
	it('lets the title decide when its home date is outside the days loaded, and marks a movie in the Library', () => {
		const facts = new Map<number, MovieFacts>([[1, { cinema: '2026-11-01', digital: '2027-01-10', have: false }], [2, { cinema: '2026-11-05', have: true }]]);
		const out = oneMovieDate([mv(1, 'cinema', '2026-11-01'), mv(2, 'cinema', '2026-11-05'), { title_id: 9, name: 'Show', kind: 'episode', date: '2026-11-02', air_day: '2026-11-02' }], facts);
		// Movie 1 is out at home in January: its cinema date in view is dropped.
		expect(out.map((x) => x.title_id)).toEqual([2, 9]);
		expect(out[0].state).toBe('in_library');
		expect(itemDot(out[0], '2026-11-10')).toBe('have');
		expect(movieFacts({ cinema_date: '2026-11-01T00:00:00Z', slots: [{ state: 'in_library' } as never] })).toMatchObject({ cinema: '2026-11-01', have: true });
	});
});

describe('filters tied to the app', () => {
	const ep = (id: number, date: string, state?: string): CalendarEntry => ({ title_id: id, name: 'X', kind: 'episode', date, air_day: date, timed: false, state });
	const mine = { asked: new Set([1]), followed: new Set([2]) };
	it('reads mine, scope and status', () => {
		expect(mineOf(ep(1, '2026-10-20'), mine)).toBe('asked');
		expect(mineOf(ep(2, '2026-10-20'), mine)).toBe('followed');
		expect(whoseOk(ep(3, '2026-10-20'), 'mine', mine)).toBe(false);
		expect(whoseOk(ep(3, '2026-10-20'), 'everyone', mine)).toBe(true);
		expect(scopeOk(ep(2, '2026-10-20'), 'asked', mine)).toBe(false);
		expect(scopeOk(ep(2, '2026-10-20'), 'followed', mine)).toBe(true);
		expect(statusOk(ep(3, '2026-10-20'), new Set(), '2026-10-21')).toBe(true);
		expect(statusOk(ep(3, '2026-10-20'), new Set(['upcoming']), '2026-10-21')).toBe(false);
		expect(statusOk(ep(3, '2026-10-20'), new Set(['missing']), '2026-10-21')).toBe(true);
	});
	it('hides past dates already in the Library by default', () => {
		expect(showingOk(ep(3, '2026-10-20', 'in_library'), 'matters', mine, '2026-10-21')).toBe(false);
		expect(showingOk(ep(3, '2026-10-20'), 'matters', mine, '2026-10-21')).toBe(true);
		expect(showingOk(ep(3, '2026-10-22', 'in_library'), 'matters', mine, '2026-10-21')).toBe(true);
		expect(showingOk(ep(3, '2026-10-20', 'in_library'), 'all', mine, '2026-10-21')).toBe(true);
		expect(showingOk(ep(3, '2026-10-22'), 'mine', mine, '2026-10-21')).toBe(false);
	});
	it('reads the filter layout in the address', () => {
		expect(filtersFromAddress('2')).toBe(2);
		expect(filtersFromAddress('9')).toBe(4);
		expect(filtersFromAddress(null, 1)).toBe(1);
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
