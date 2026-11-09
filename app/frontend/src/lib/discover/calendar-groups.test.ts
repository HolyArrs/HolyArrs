import { dayChip, dayName, entryTag, entryTitle, groupDates, groupTag, monthGroups, thingCount } from './calendar-groups.ts';
import { describe, expect, it } from 'vitest';
import type { CalendarEntry } from '../core/types.ts';

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
		expect(entryTag(movie('physical', '2026-12-18'))).toBe('New movie');
		expect(entryTag(album('2026-10-09', { album_type: 'EP' }))).toBe('New EP');
		expect(entryTag(album('2026-10-09', { album_type: 'Single' }))).toBe('New single');
	});
	it('puts the code, time and network on one facts line, and skips generic episode names', () => {
		expect(entryTitle(album('2026-10-09'))).toBe('Mud');
	});
});

describe('groupings', () => {
	it('splits days by month in order', () => {
		const g = monthGroups([ep({ air_day: '2026-11-02', date: '2026-11-02T00:00:00Z', item_id: 2 }), ep()]);
		expect(g.map((m) => m.month)).toEqual(['2026-10', '2026-11']);
	});
});

describe('busy days', () => {
	it('counts things', () => {
		expect(thingCount(1)).toBe('1 coming out');
		expect(thingCount(10)).toBe('10 coming out');
	});
	it('folds a show episodes on one day into one card, keeping other shows apart', () => {
		const a = ep({ title_id: 2, name: 'Home and Away', season: 39, episode: 194, air_day: '2026-10-29' });
		const b = ep({ title_id: 2, name: 'Home and Away', season: 39, episode: 196, air_day: '2026-10-29' });
		const c = ep({ title_id: 2, name: 'Home and Away', season: 39, episode: 195, air_day: '2026-10-29' });
		const d = ep({ title_id: 2, name: 'Home and Away', season: 39, episode: 197, air_day: '2026-10-30' });
		const g = groupDates([a, ep(), b, c, d]);
		expect(g.map((x) => x.entries.length)).toEqual([3, 1, 1]);
		expect(groupTag(g[0])).toBe('3 episodes');
		expect(groupTag(g[1])).toBe('New episode');
	});
	it('folds a movie dates on one day, and names them', () => {
		const g = groupDates([movie('digital', '2026-11-17'), movie('physical', '2026-11-17'), album('2026-11-17')]);
		expect(g).toHaveLength(2);
		expect(groupTag(g[0])).toBe('Digital and disc');
	});
});
