import { genrePath, genresFor, parseGenre } from './genres.ts';
import { serviceChips, serviceLabel, serviceTopPath, topPath, type ServiceTopRow } from './explore-services.ts';
import { describe, expect, it } from 'vitest';

describe('Trending on streaming', () => {
	it('asks for one chip, with the services, region and genre', () => {
		expect(topPath({ type: 'movies', chip: 'all', services: [8, 337], region: 'US' })).toBe('/explore/top?type=movies&chip=all&region=US&services=8%7C337');
		expect(topPath({ type: 'shows', chip: 8, services: [8], region: 'GB', genre: 18 })).toBe('/explore/top?type=shows&chip=8&region=GB&services=8&genre=18');
		expect(topPath({ type: 'all', chip: 8, services: [], region: 'US', genre: 27 })).toBe('/explore/top?type=all&chip=8&region=US');
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
		expect(parseGenre('878', 'all')?.name).toBe('Science Fiction');
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
