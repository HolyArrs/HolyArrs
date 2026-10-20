import { describe, expect, it } from 'vitest';
import { headPath, serverRow, tailPath } from './explore.ts';

describe('server-built Explore', () => {
	it('builds the head and tail paths', () => {
		expect(headPath({ type: 'movies', services: [8, 9], region: 'US', top10Kind: 'movie', top10Region: 'US' })).toBe(
			'/explore/head?type=movies&top10_kind=movie&top10_region=US&services=8%7C9&region=US'
		);
		expect(headPath({ type: 'all' })).toBe('/explore/head?type=all');
		expect(tailPath({ type: 'shows' }, 2)).toBe('/explore/tail?type=shows&page=2');
	});
	it('keeps wide pictures for Coming soon only', () => {
		const item = { tmdb_id: '1', kind: 'movie' as const, name: 'A', backdrop: 'b.jpg', poster: 'p.jpg', in_library: false };
		const soon = serverRow({ id: 'coming_soon', type: 'coming_soon', title: 'Coming soon', wide: true, items: [item] });
		expect(soon.wide).toBe(true);
		expect(soon.cards[0].wide).toBe('b.jpg');
		const other = serverRow({ id: 'best', type: 'best', title: 'Best new releases', items: [item] });
		expect(other.cards[0].wide).toBeUndefined();
		expect(other.cards[0].ref).toBe('tmdb:1');
	});
});

describe('revision 4', () => {
	it('keeps only factual badges', async () => {
		const { factualBadges } = await import('./explore.ts');
		expect(factualBadges(['New season', 'Leaving soon', '3 days left', 'Coming 12 Nov', 'In library', 'Asked', '4K'])).toEqual(['New season', 'Coming 12 Nov', 'In library', 'Asked', '4K']);
	});
	it('keeps the Top 10 marker row in place', () => {
		const r = serverRow({ id: 'top10', type: 'top10', title: 'Top 10 today', items: [] });
		expect(r.keep).toBe(true);
	});
});
