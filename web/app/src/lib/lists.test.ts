import { describe, expect, it } from 'vitest';
import { every, listLink } from './lists.ts';

describe('outside lists', () => {
	it('links each list to its own site', () => {
		expect(listLink('tmdb_list', '42')).toBe('https://www.themoviedb.org/list/42');
		expect(listLink('trakt_list', 'ana/favourites')).toBe('https://trakt.tv/users/ana/lists/favourites');
		expect(listLink('trakt_list', 'ana/watchlist')).toBe('https://trakt.tv/users/ana/watchlist');
		expect(listLink('trakt_list', 'broken')).toBe('');
	});
	it('says how often a list is read', () => {
		expect(every(12)).toBe('every 12 hours');
		expect(every(24)).toBe('every day');
		expect(every(72)).toBe('every 3 days');
	});
});
