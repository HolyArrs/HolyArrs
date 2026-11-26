// Badges, sorting and the name filter for the release lists.
import { describe, expect, it } from 'vitest';
import { nameMatches, qualityRank, readTags, resolutionsOf, sortReleases } from './releases.ts';
import type { ReleaseView } from '#lib/core/types.ts';

const rel = (o: Partial<ReleaseView>): ReleaseView => ({ id: 1, name: 'x', size: 1, protocol: 'torrent', state: 'judged', found_by: 'search', source_id: 1, source_name: 's', outcome: 'passed_over', passed: true, rank: 1, points: 0, good_enough: true, needs_force: false, reasons: [], ...o }) as ReleaseView;

describe('readTags', () => {
	it('shows what the server read, not a guess from the name', () => {
		// The name says UHD but the judge read 1080p: the badge must say 1080p.
		expect(readTags(rel({ name: 'Movie.UHDBD.2160.REMUX', read: { resolution: 1080, source: 'remux' } }))).toEqual(['1080p', 'Remux']);
		expect(readTags(rel({ read: { resolution: 2160, source: 'bluray', hdr: ['dv', 'hdr10'], codec: 'x265', atmos: true, languages: ['fr'], proper: true } }))).toEqual(['4K', 'BluRay', 'Dolby Vision', 'HDR10', 'x265', 'Atmos', 'FR', 'PROPER']);
		expect(readTags(rel({ read: { format: 'flac', bit_depth: 24, lossless: true } }))).toEqual(['FLAC', '24-bit']);
		expect(readTags(rel({}))).toEqual([]);
	});
});

describe('sortReleases', () => {
	const a = rel({ id: 1, rank: 2, size: 5, seeders: 1, published_at: '2026-01-01T00:00:00Z', read: { resolution: 1080, source: 'web_dl' } });
	const b = rel({ id: 2, rank: 1, size: 1, seeders: 9, published_at: '2025-01-01T00:00:00Z', read: { resolution: 2160, source: 'remux' } });
	const c = rel({ id: 3, passed: false, rank: 0, size: 9, read: { resolution: 1080, source: 'remux' } });
	const ids = (l: ReleaseView[]) => l.map((r) => r.id);
	it('orders by each field', () => {
		expect(ids(sortReleases([a, b, c], 'best'))).toEqual([2, 1, 3]);
		expect(ids(sortReleases([a, b, c], 'quality'))).toEqual([2, 3, 1]);
		expect(ids(sortReleases([a, b, c], 'size'))).toEqual([3, 1, 2]);
		expect(ids(sortReleases([a, b, c], 'seeders'))).toEqual([2, 1, 3]);
		expect(ids(sortReleases([a, b, c], 'age'))).toEqual([1, 2, 3]);
		expect(qualityRank(b)).toBeGreaterThan(qualityRank(c));
		expect(resolutionsOf([a, b, c])).toEqual([2160, 1080]);
	});
});

describe('nameMatches', () => {
	it('matches words across dots and dashes', () => {
		expect(nameMatches('Movie.2019.2160p.WEB-DL-GRP', 'web dl grp')).toBe(true);
		expect(nameMatches('Movie.2019.2160p.WEB-DL-GRP', '1080p')).toBe(false);
		expect(nameMatches('Movie', '  ')).toBe(true);
	});
});
