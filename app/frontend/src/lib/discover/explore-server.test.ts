import { headPath, serverRow, tailPath } from './explore-server.ts';
import { describe, expect, it } from 'vitest';

describe('server-built Explore', () => {
	it('builds the head and tail paths', () => {
		expect(headPath({ type: 'movies', services: [8, 9], region: 'US' })).toBe(
			'/explore/head?type=movies&services=8%7C9&region=US'
		);
		expect(headPath({ type: 'all' })).toBe('/explore/head?type=all');
		expect(tailPath({ type: 'shows' }, 2)).toBe('/explore/tail?type=shows&page=2');
	});
	it('keeps the wide picture on every card (wide title cards)', () => {
		const item = { tmdb_id: '1', kind: 'movie' as const, name: 'A', backdrop: 'b.jpg', poster: 'p.jpg', in_library: false };
		const soon = serverRow({ id: 'coming_soon', type: 'coming_soon', title: 'Coming soon', wide: true, items: [item] });
		expect(soon.wide).toBe(true);
		expect(soon.cards[0].wide).toBe('b.jpg');
		const other = serverRow({ id: 'best', type: 'best', title: 'Best new releases', items: [item] });
		expect(other.wide).toBe(false);
		expect(other.cards[0].wide).toBe('b.jpg');
		expect(other.cards[0].ref).toBe('tmdb:1');
	});
	it('carries the server title, subtitle, layer and time zone', () => {
		expect(tailPath({ type: 'all', tz: 'Europe/Paris' }, 0)).toBe('/explore/tail?type=all&tz=Europe%2FParis&page=0');
		const r = serverRow({ id: 'genre:Horror', type: 'genre', title: 'Late-night horror', subtitle: 'Horror picks for you', title_layer: 'time', items: [] });
		expect(r).toMatchObject({ title: 'Late-night horror', subtitle: 'Horror picks for you', titleLayer: 'time' });
		expect(serverRow({ id: 'best', type: 'best', title: 'Best new releases', items: [] }).subtitle).toBeUndefined();
	});
});

describe('revision 4', () => {
	it('keeps only factual badges', async () => {
		const { factualBadges } = await import('./explore-server.ts');
		expect(factualBadges(['New season', 'Leaving soon', '3 days left', 'Coming 12 Nov', 'In library', 'Asked', '4K'])).toEqual(['New season', 'Coming 12 Nov', 'In library', 'Asked', '4K']);
	});
	it('keeps the Top on your services marker row in place', () => {
		const r = serverRow({ id: 'top_services', type: 'top_services', title: 'Trending on streaming', items: [] });
		expect(r.keep).toBe(true);
		expect(serverRow({ id: 'top10', type: 'top10', title: 'Trending now', items: [] }).keep).toBe(false);
	});
});
