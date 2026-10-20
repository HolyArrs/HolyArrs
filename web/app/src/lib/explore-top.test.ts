import { describe, expect, it } from 'vitest';
import { genrePath, genresFor, parseGenre, serviceChips, serviceLabel, serviceTopPath, topChips, topPath, topSource, type ServiceTopRow } from './explore.ts';

describe('Trending on streaming', () => {
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
		expect(topSource({ source: 'netflix_top10', region: 'US' }, 'Netflix')).toBe("Trending now on Netflix · US");
		expect(topSource({ source: 'popular', region: 'US' }, 'Disney+')).toBe('Popular on Disney+ · US');
		expect(topSource({ source: 'popular', region: 'US' }, 'your services')).toBe('Popular on your services · US');
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

describe('per-service top lists', () => {
	const row = (service: number, name: string, n = 1): ServiceTopRow => ({ service, key: name.toLowerCase(), name, source: 'streaming-availability', fetched_at: '2026-10-18T10:00:00Z', age_seconds: 60, stale: false, ranked: true, items: Array.from({ length: n }, (_, i) => ({ tmdb_id: String(i + 1), kind: 'movie' as const, name: 'T', in_library: false, rank: i + 1 })) });
	it('asks once for every service', () => {
		expect(serviceTopPath('movies', 'US')).toBe('/explore/service-top?type=movies&region=US');
		expect(serviceTopPath('music', 'GB')).toBe('/explore/service-top?type=all&region=GB');
	});
	it("puts the person's services first and leaves out empty lists", () => {
		const rows = [row(8, 'Netflix'), row(9, 'Prime Video'), row(337, 'Disney+'), row(386, 'Peacock', 0)];
		expect(serviceChips(rows, [337, 9]).map((r) => r.name)).toEqual(['Disney+', 'Prime Video', 'Netflix']);
		expect(serviceChips(rows, []).map((r) => r.service)).toEqual([8, 9, 337]);
	});
	it('labels today, an older list, and TMDB popularity', () => {
		expect(serviceLabel(row(8, 'Netflix'))).toBe('Trending now on Netflix');
		expect(serviceLabel({ ...row(8, 'Netflix'), age_seconds: 2 * 86400 })).toBe('Trending on Netflix as of Oct 18');
		expect(serviceLabel({ ...row(386, 'Peacock'), source: 'tmdb-popular' })).toBe('Popular on Peacock');
		expect(serviceLabel(row(43, 'Starz'))).toBe('Trending now on Starz');
		expect(serviceLabel({ ...row(73, 'Tubi'), source: 'tmdb-popular' })).toBe('Popular on Tubi');
	});
});
