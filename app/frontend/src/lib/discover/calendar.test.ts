import { addDays, rangeChunks, titlesFromAddress, itemDot, oneMovieDate, movieFacts, followsOk, followerNames, withFollow, startFollows, readFollows, saveFollows, type MovieFacts, byDay, monthGrid, monthRange, shiftMonth, ymd, weekStart, readView, saveView, startView, viewFromAddress, entryArt, bannerArt, type BannerEntry } from './calendar.ts';
import { describe, expect, it } from 'vitest';
import type { CalendarEntry } from '../core/types.ts';

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
});

describe('calendar week', () => {
	it('starts weeks on Monday', () => {
		const s = weekStart(new Date(2026, 9, 18)); // A Sunday.
		expect(ymd(s)).toBe('2026-10-12');
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
		expect(oneMovieDate([mv(3, 'digital', '2026-12-01'), mv(3, 'physical', '2026-12-01')])).toHaveLength(1);
	});
	it('lets the title decide when its home date is outside the days loaded, and marks a movie in the Library', () => {
		const facts = new Map<number, MovieFacts>([[1, { cinema: '2026-11-01', digital: '2027-01-10', have: false }], [2, { cinema: '2026-11-05', have: true }]]);
		const out = oneMovieDate([mv(1, 'cinema', '2026-11-01'), mv(2, 'cinema', '2026-11-05'), { title_id: 9, name: 'Show', kind: 'episode', date: '2026-11-02', air_day: '2026-11-02' }], facts);
		expect(out.map((x) => x.title_id)).toEqual([2, 9]);
		expect(out[0].state).toBe('in_library');
		expect(itemDot(out[0], '2026-11-10')).toBe('have');
		expect(movieFacts({ cinema_date: '2026-11-01T00:00:00Z', slots: [{ state: 'in_library' } as never] })).toMatchObject({ cinema: '2026-11-01', have: true });
	});
});

describe('follows', () => {
	const ep = (id: number, extra: Partial<CalendarEntry> = {}): CalendarEntry => ({ title_id: id, name: 'X', kind: 'episode', date: '2026-10-20', air_day: '2026-10-20', ...extra });
	const mineE = ep(1, { following: true, followers: [{ id: 1, name: 'luis', me: true }] });
	const anaE = ep(2, { followers: [{ id: 2, name: 'Ana' }] });
	const hiddenE = ep(3, { followers_hidden: 2 });
	const noneE = ep(4);
	it('shows what I follow, what anyone follows, or the whole Library', () => {
		expect([mineE, anaE, hiddenE, noneE].filter((e) => followsOk(e, 'mine')).map((e) => e.title_id)).toEqual([1]);
		expect([mineE, anaE, hiddenE, noneE].filter((e) => followsOk(e, 'everyone')).map((e) => e.title_id)).toEqual([1, 2, 3]);
		expect([mineE, anaE, hiddenE, noneE].filter((e) => followsOk(e, 'library')).map((e) => e.title_id)).toEqual([1, 2, 3, 4]);
	});
	it('says who follows', () => {
		expect(followerNames(mineE)).toBe('You');
		expect(followerNames(hiddenE)).toBe('2 people');
		expect(followerNames(ep(5, { followers: [{ id: 1, name: 'luis', me: true }, { id: 2, name: 'Ana' }], followers_hidden: 1 }))).toBe('You, Ana and 1 other');
		expect(followerNames(noneE)).toBe('');
	});
	it('follows and unfollows in place', () => {
		const on = withFollow(anaE, true, { name: 'luis' });
		expect(on.following).toBe(true);
		expect(on.followers?.map((p) => p.name)).toEqual(['Ana', 'luis']);
		const off = withFollow(on, false, { name: 'luis' });
		expect(off.following).toBe(false);
		expect(off.followers?.map((p) => p.name)).toEqual(['Ana']);
	});
	it('opens on My follows, else the Whole Library, and keeps the pick for the session', () => {
		expect(startFollows(null, 3, 2)).toBe('mine');
		expect(startFollows(null, 0, 2)).toBe('library');
		expect(startFollows('everyone', 3, 2)).toBe('everyone');
		expect(startFollows('everyone', 3, 1)).toBe('mine');
		expect(startFollows('library', 3, 1)).toBe('library');
		const m = new Map<string, string>();
		const store = { getItem: (k: string) => m.get(k) ?? null, setItem: (k: string, v: string) => void m.set(k, v) };
		expect(readFollows(store)).toBeNull();
		saveFollows('everyone', store);
		expect(readFollows(store)).toBe('everyone');
		const broken = { getItem: () => { throw new Error('no'); }, setItem: () => { throw new Error('no'); } };
		expect(readFollows(broken)).toBeNull();
		expect(() => saveFollows('mine', broken)).not.toThrow();
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

describe('bannerArt', () => {
	it('uses the textless banner and logo, then the backdrop and still', () => {
		const ep: BannerEntry = { title_id: 1, name: 'X', kind: 'episode', date: '2026-10-20', banner: 'b', logo: 'l', backdrop: 'd', still: 's' };
		expect(bannerArt(ep)).toEqual({ images: ['b', 'd', 's'], logo: 'l' });
		expect(bannerArt({ ...ep, banner: undefined, logo: undefined })).toEqual({ images: ['d', 's'], logo: null });
	});
	it('music: the artist wide picture, else the cover', () => {
		expect(bannerArt({ title_id: 2, name: 'A', kind: 'album', date: '2026-10-20', banner: 'w', poster: 'c', still: 's' }).images).toEqual(['w', 'c']);
	});
});
