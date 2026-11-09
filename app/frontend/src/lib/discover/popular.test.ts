import { cardServices, dayOffset, hasServiceChips, isRanked, leaveOut, newHref, newPath, newServicePath, newServicesPath, parseNewType, pickService, railLabel } from './popular.ts';
import { describe, expect, it } from 'vitest';

describe('newServicePath', () => {
	it('names the chip row for the server', () => {
		expect(newServicePath('all', 8, 'np_services')).toBe('/explore/new/service?id=8&type=all&row=services');
		expect(newServicePath('shows', 337, 'np_top_shows')).toBe('/explore/new/service?id=337&type=shows&row=hot_shows');
		expect(newServicePath('movies', 9, 'np_top_movies')).toBe('/explore/new/service?id=9&type=movies&row=hot_movies');
	});
});

const card = (id: number, kind: 'movie' | 'show' = 'movie') => ({ key: `r-${kind}-${id}`, kind, ref: `tmdb:${id}`, name: `T${id}` });

describe('leaveOut', () => {
	const cards = [1, 2, 3, 4, 5, 6, 7, 8].map((i) => card(i));
	it('drops titles the page already shows, by identity or kind:tmdb key', () => {
		const got = leaveOut(cards, ['movie:tmdb:1', 'movie:2']);
		expect(got.map((c) => c.ref)).toEqual(['tmdb:3', 'tmdb:4', 'tmdb:5', 'tmdb:6', 'tmdb:7', 'tmdb:8']);
	});
	it('keeps a movie and a show with the same id apart', () => {
		expect(leaveOut([...cards.slice(0, 6), card(1, 'show')], ['show:1'])).toHaveLength(6);
	});
	it('hides the row under six', () => {
		expect(leaveOut(cards, ['movie:1', 'movie:2', 'movie:3'])).toEqual([]);
	});
});

describe('paths and helpers', () => {
	it('builds paths and reads the type', () => {
		expect(newPath('shows', 2)).toBe('/explore/new?type=shows&page=2');
		expect(newServicesPath('music')).toBe('/explore/new/services?type=all');
		expect(newHref('movies')).toBe('/popular?type=movies');
		expect(newHref('music')).toBe('/popular');
		expect(parseNewType('x')).toBe('all');
		expect(parseNewType('music')).toBe('music');
		expect(isRanked('np_top_shows')).toBe(true);
	});
	it('lists services once, in order', () => {
		const s = cardServices([{ streaming: { service: 8, name: 'Netflix' } }, {}, { streaming: { service: 8, name: 'Netflix' } }, { streaming: { service: 337, name: 'Disney+' } }]);
		expect(s.map((x) => x.service)).toEqual([8, 337]);
	});
});

describe('service chips', () => {
	const cs = [
		{ streaming: { service: 8, name: 'Netflix', rank: 1 } },
		{ streaming: { service: 337, name: 'Disney+' } },
		{},
		{ streaming: { service: 8, name: 'Netflix', rank: 2 } }
	];
	it('services and both hot and new rows draw chips', () => {
		expect(hasServiceChips('np_services')).toBe(true);
		expect(hasServiceChips('np_top_shows')).toBe(true);
		expect(hasServiceChips('np_picks')).toBe(false);
	});
	it('a chip keeps that service in order; all keeps everything', () => {
		expect(pickService(cs, 8).map((c) => c.streaming?.rank)).toEqual([1, 2]);
		expect(pickService(cs, null)).toHaveLength(4);
	});
});

describe('This week rail', () => {
	const today = new Date(2026, 9, 21); // Wed Oct 21 (local).
	it('counts days from today', () => {
		expect(dayOffset('2026-10-21', today)).toBe(0);
		expect(dayOffset('2026-10-18', today)).toBe(-3);
		expect(dayOffset('2026-10-28', today)).toBe(7);
		expect(dayOffset('2026-11-01', today)).toBe(11);
	});
	it('names the near days and the weekday after', () => {
		expect(railLabel('2026-10-21', today)).toEqual({ big: 'Today', small: 'Wed Oct 21' });
		expect(railLabel('2026-10-20', today).big).toBe('Yesterday');
		expect(railLabel('2026-10-22', today).big).toBe('Tomorrow');
		expect(railLabel('2026-10-24', today)).toEqual({ big: 'Sat', small: 'Oct 24' });
	});
});