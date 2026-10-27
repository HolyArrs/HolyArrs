import { describe, expect, it } from 'vitest';
import type { CalendarEntry } from './types.ts';
import { dayChip, dayDots, dayName, entryFacts, entryLine, entryTag, entryTitle, fitThumbs, groupDates, groupFacts, groupTag, monthGroups, movieDates, nextBusy, readSpan, shelves, spanName, startDay, stripDays, thingCount, weekTitle } from './calendar-looks.ts';

const ep = (o: Partial<CalendarEntry> = {}): CalendarEntry => ({ title_id: 1, name: "Grey's Anatomy", kind: 'episode', item_id: 9, season: 23, episode: 3, episode_name: 'Collide', air_day: '2026-10-29', date: '2026-10-29T00:00:00Z', network: 'ABC', ...o });
const movie = (kind: CalendarEntry['kind'], date: string, o: Partial<CalendarEntry> = {}): CalendarEntry => ({ title_id: 7, name: 'Dune: Part Three', year: 2026, kind, date: `${date}T00:00:00Z`, ...o });
const album = (date: string, o: Partial<CalendarEntry> = {}): CalendarEntry => ({ title_id: 3, part_id: 4, name: 'Remi Wolf', album: 'Mud', album_type: 'Album', tracks: 12, kind: 'album', date: `${date}T00:00:00Z`, ...o });

describe('day words', () => {
	it('names today, tomorrow and yesterday, else the date', () => {
		expect(dayName('2026-10-27', '2026-10-27')).toBe('Today');
		expect(dayName('2026-10-28', '2026-10-27')).toBe('Tomorrow');
		expect(dayName('2026-10-26', '2026-10-27')).toBe('Yesterday');
		expect(dayName('2026-10-29', '2026-10-27')).toMatch(/29/);
		expect(dayChip('2026-10-29', '2026-10-27')).toMatch(/ 29$/);
	});
});

describe('card words', () => {
	it('tags new episodes, premieres, movie dates and albums', () => {
		expect(entryTag(ep())).toBe('New episode');
		expect(entryTag(ep({ episode: 1 }))).toBe('Season premiere');
		expect(entryTag(ep({ season: 1, episode: 1 }))).toBe('Show premiere');
		expect(entryTag(movie('cinema', '2026-12-18'))).toBe('In cinemas');
		expect(entryTag(movie('physical', '2026-12-18'))).toBe('Disc release');
		expect(entryTag(album('2026-10-09', { album_type: 'EP' }))).toBe('New EP');
		expect(entryTag(album('2026-10-09', { album_type: 'Single' }))).toBe('New single');
	});
	it('puts the code, time and network on one facts line, and skips generic episode names', () => {
		expect(entryFacts(ep())).toBe('S23E03 · ABC');
		expect(entryFacts(ep({ timed: true, date: new Date(2026, 9, 29, 20, 0).toISOString() }))).toBe('S23E03 · 20:00 · ABC');
		expect(entryLine(ep({ episode_name: 'Episode 4' }))).toBe('');
		expect(entryLine(ep())).toBe('Collide');
		expect(entryTitle(album('2026-10-09'))).toBe('Mud');
		expect(entryLine(album('2026-10-09'))).toBe('Remi Wolf');
		expect(entryFacts(album('2026-10-09'))).toBe('Album · 12 tracks');
	});
	it('lists a movie’s other loaded dates', () => {
		const all = [movie('cinema', '2026-12-18'), movie('digital', '2027-02-02'), movie('physical', '2027-03-09'), ep()];
		const s = movieDates(all[0], all);
		expect(s).toMatch(/^Digital .* · Disc /);
		expect(movieDates(ep(), all)).toBe('');
	});
});

describe('groupings', () => {
	it('splits days by month in order', () => {
		const g = monthGroups([ep({ air_day: '2026-11-02', date: '2026-11-02T00:00:00Z', item_id: 2 }), ep()]);
		expect(g.map((m) => m.month)).toEqual(['2026-10', '2026-11']);
	});
	it('builds the strip with items, today and the next busy day', () => {
		const days = stripDays(new Date(2026, 9, 26), 14, [ep()], '2026-10-27');
		expect(days).toHaveLength(14);
		expect(days[1].today).toBe(true);
		expect(days[0].past).toBe(true);
		expect(days[3].items).toHaveLength(1);
		expect(startDay(days)).toBe('2026-10-27');
		expect(nextBusy('2026-10-27', days)).toBe('2026-10-29');
		expect(nextBusy('2026-10-29', days)).toBeNull();
		expect(startDay(stripDays(new Date(2027, 2, 1), 7, [], '2026-10-27'))).toBe('2027-03-01');
	});
	it('fills shelves by when and by kind, leaving empty rows out', () => {
		const rows = shelves(
			[
				ep({ air_day: '2026-10-27', item_id: 1 }),
				ep({ air_day: '2026-10-29', item_id: 2 }),
				ep({ air_day: '2026-11-03', item_id: 3 }),
				ep({ air_day: '2026-12-07', item_id: 4 }),
				movie('digital', '2026-11-05'),
				album('2026-10-30')
			],
			'2026-10-27'
		);
		expect(rows.map((r) => r.title)).toEqual(['Today', 'This week', 'Next week', 'In December', 'Coming to digital', 'New albums']);
		expect(rows.find((r) => r.key === 'albums')?.shape).toBe('square');
		expect(shelves([], '2026-10-27')).toEqual([]);
	});
	it('gives one dot per kind of date', () => {
		expect(dayDots([ep(), movie('cinema', '2026-10-29'), movie('digital', '2026-10-29')])).toEqual(['episode', 'movie']);
		expect(dayDots([])).toEqual([]);
	});
});

describe('Coming Soon spans', () => {
	it('reads the span from the address, day when missing or unknown', () => {
		expect(readSpan('week')).toBe('week');
		expect(readSpan('month')).toBe('month');
		expect(readSpan('day')).toBe('day');
		expect(readSpan(null)).toBe('day');
		expect(readSpan('year')).toBe('day');
	});
	it('names a week by its first and last day, a month by its name', () => {
		const w = spanName('week', new Date(2026, 9, 26), 2026, 9);
		expect(w).toMatch(/26/);
		expect(w).toMatch(/1/);
		expect(w).toMatch(/2026/);
		expect(spanName('month', new Date(2026, 9, 26), 2026, 10)).toMatch(/2026/);
		expect(spanName('month', new Date(2026, 9, 26), 2026, 10)).not.toBe(spanName('month', new Date(2026, 9, 26), 2026, 9));
	});
	it('fits as many pictures as the room holds, keeping room for +N', () => {
		expect(fitThumbs([30, 30], 100, 4, 28)).toBe(2);
		expect(fitThumbs([30, 30, 30], 98, 4, 28)).toBe(3);
		expect(fitThumbs([30, 30, 30], 97, 4, 28)).toBe(2);
		// Four do not fit: two pictures and the chip do (30+4+30+4+28 = 96).
		expect(fitThumbs([30, 30, 30, 30], 98, 4, 28)).toBe(2);
		expect(fitThumbs([30, 30, 30, 30], 60, 4, 28)).toBe(0);
		expect(fitThumbs([], 60, 4, 28)).toBe(0);
		expect(fitThumbs([45, 30], 80, 4, 28)).toBe(2);
	});
});

describe('busy days', () => {
	it('counts things', () => {
		expect(thingCount(1)).toBe('1 thing');
		expect(thingCount(10)).toBe('10 things');
	});
	it('folds a show episodes on one day into one card, keeping other shows apart', () => {
		const a = ep({ title_id: 2, name: 'Home and Away', season: 39, episode: 194, air_day: '2026-10-29' });
		const b = ep({ title_id: 2, name: 'Home and Away', season: 39, episode: 196, air_day: '2026-10-29' });
		const c = ep({ title_id: 2, name: 'Home and Away', season: 39, episode: 195, air_day: '2026-10-29' });
		const d = ep({ title_id: 2, name: 'Home and Away', season: 39, episode: 197, air_day: '2026-10-30' });
		const g = groupDates([a, ep(), b, c, d]);
		expect(g.map((x) => x.entries.length)).toEqual([3, 1, 1]);
		expect(groupTag(g[0])).toBe('3 episodes');
		expect(groupFacts(g[0])).toMatch(/^S39E194–E196/);
		expect(groupTag(g[1])).toBe('New episode');
	});
	it('folds a movie dates on one day, and names them', () => {
		const g = groupDates([movie('digital', '2026-11-17'), movie('physical', '2026-11-17'), album('2026-11-17')]);
		expect(g).toHaveLength(2);
		expect(groupTag(g[0])).toBe('Digital and disc');
	});
	it('heads week rows', () => {
		expect(weekTitle(new Date(2026, 9, 26), '2026-10-27')).toMatch(/^This week · /);
		expect(weekTitle(new Date(2026, 10, 2), '2026-10-27')).toMatch(/^Next week · /);
		expect(weekTitle(new Date(2026, 10, 9), '2026-10-27')).not.toMatch(/week/);
	});
});
