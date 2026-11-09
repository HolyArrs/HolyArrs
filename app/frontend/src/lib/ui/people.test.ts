import { describe, expect, it, vi } from 'vitest';
import { byDecade, normalizeSelection, personChips, runReviewed } from './people.ts';

describe('people filmography actions', () => {
	it('keeps stable unique TMDB ids in review order', () => {
		expect(normalizeSelection(['103', ' 102 ', '103', '', 'x', '001'])).toEqual(['103', '102', '001']);
	});

	it('uses existing individual routes and preserves per-item failures', async () => {
		const post = vi.fn(async (_path: string, body: Record<string, unknown>) => {
			if (body.id === 'tmdb:101') throw new Error('folder needs attention');
			return { ok: true };
		});
		const add = await runReviewed(post, 'add', ['101', '102']);
		expect(post).toHaveBeenNthCalledWith(1, '/titles', { id: 'tmdb:101' });
		expect(add).toEqual([
			{ tmdb_id: '101', ok: false, error: 'folder needs attention' },
			{ tmdb_id: '102', ok: true }
		]);

		post.mockReset();
		post.mockResolvedValue({ ok: true });
		const ask = await runReviewed(post, 'ask', ['103', '103']);
		expect(post).toHaveBeenCalledTimes(1);
		expect(post).toHaveBeenCalledWith('/asks', { id: 'tmdb:103' });
		expect(ask).toEqual([{ tmdb_id: '103', ok: true }]);
	});

});

describe('person page helpers', () => {
	it('groups credits by decade in arrival order, undated last', () => {
		const m = (year: number, tmdb: string) => ({ name: tmdb, year, overview: '', ids: { tmdb } });
		const g = byDecade([m(2004, 'a'), m(2019, 'b'), m(2012, 'c'), m(0, 'd')]);
		expect(g.map((x) => x.label)).toEqual(['2000s', '2010s', 'Coming soon']);
		expect(g[1].movies.map((x) => x.ids.tmdb)).toEqual(['b', 'c']);
	});
	it('builds at most four hero chips and hides empty ones', () => {
		expect(personChips({ first_year: 2001, last_year: 2026, active: true, films: 52, rating: 6.84, rated: 30 }, 4, 1)).toEqual(['2001–now', '52 films', '★ 6.8 avg', '4 in your Library · 1 asked']);
		expect(personChips({ first_year: 1990, last_year: 1999, active: false, films: 1 }, 0, 0)).toEqual(['1990–1999', '1 film']);
		expect(personChips({ active: true, films: 52, shows: 16, titles: 68 }, 0, 0)).toEqual(['68 titles · 52 films · 16 shows']);
		expect(personChips({ active: false, films: 0, shows: 1 }, 0, 0)).toEqual(['1 title · 1 show']);
	});
});
