import { describe, expect, it } from 'vitest';
import { genrePath, genresFor, parseGenre, topChips, topPath, topSource } from './explore.ts';

describe('Top on your services', () => {
	it('asks for one chip, with the services, region and genre', () => {
		expect(topPath({ type: 'movies', chip: 'all', services: [8, 337], region: 'US' })).toBe('/explore/top?type=movies&chip=all&region=US&services=8%7C337');
		expect(topPath({ type: 'shows', chip: 8, services: [8], region: 'GB', genre: 18 })).toBe('/explore/top?type=shows&chip=8&region=GB&services=8&genre=18');
		// Under All a genre is never sent.
		expect(topPath({ type: 'all', chip: 8, services: [], region: 'US', genre: 27 })).toBe('/explore/top?type=all&chip=8&region=US');
	});
	it('offers All my services, then each picked service', () => {
		const shown = [{ id: 8, name: 'Netflix', priority: 1 }, { id: 337, name: 'Disney Plus', priority: 2 }, { id: 9, name: 'Prime Video', priority: 3 }];
		expect(topChips([337, 8, 4040], shown).map((c) => c.name)).toEqual(['All my services', 'Disney Plus', 'Netflix']);
		expect(topChips([], shown).map((c) => c.id)).toEqual(['all']);
	});
	it('names the source honestly', () => {
		expect(topSource({ source: 'netflix_top10', region: 'US' }, 'Netflix')).toBe("Netflix's weekly Top 10 · US");
		expect(topSource({ source: 'popular', region: 'US' }, 'Disney+')).toBe('Most popular this week on Disney+ · US');
		expect(topSource({ source: 'popular', region: 'US' }, 'your services')).toBe('Most popular this week on your services · US');
	});
});

describe('genre pages', () => {
	it('lists genres only on Movies and Shows', () => {
		expect(genresFor('movies').find((g) => g.id === 27)?.name).toBe('Horror');
		expect(genresFor('shows').some((g) => g.id === 27)).toBe(false);
		expect(genresFor('all')).toEqual([]);
		expect(genresFor('music')).toEqual([]);
	});
	it('reads ?genre= for the tab', () => {
		expect(parseGenre('27', 'movies')?.name).toBe('Horror');
		expect(parseGenre('27', 'shows')).toBeNull();
		expect(parseGenre('10765', 'shows')?.name).toBe('Sci-Fi & Fantasy');
		expect(parseGenre(null, 'movies')).toBeNull();
		expect(parseGenre('27', 'all')).toBeNull();
	});
	it('builds the genre page path', () => {
		expect(genrePath('movies', 27)).toBe('/explore/genre?type=movies&genre=27');
	});
});
