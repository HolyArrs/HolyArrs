import { describe, expect, it } from 'vitest';
import { cardServices, isRanked, leaveOut, newHref, newPath, newServicesPath, parseNewType } from './new-popular.ts';

const card = (id: number, kind: 'movie' | 'show' = 'movie') => ({ key: `r-${kind}-${id}`, kind, ref: `tmdb:${id}`, name: `T${id}` });

describe('leaveOut', () => {
	const cards = [1, 2, 3, 4, 5, 6, 7, 8].map((i) => card(i));
	it('drops titles the page already shows, by identity or Top 10 key', () => {
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
		expect(newHref('movies')).toBe('/new?type=movies');
		expect(newHref('music')).toBe('/new');
		expect(parseNewType('x')).toBe('all');
		expect(isRanked('np_top_shows')).toBe(true);
	});
	it('lists services once, in order', () => {
		const s = cardServices([{ streaming: { service: 8, name: 'Netflix' } }, {}, { streaming: { service: 8, name: 'Netflix' } }, { streaming: { service: 337, name: 'Disney+' } }]);
		expect(s.map((x) => x.service)).toEqual([8, 337]);
	});
});
