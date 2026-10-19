import { describe, expect, it } from 'vitest';
import { ApiError } from './api.ts';
import { defaultRegion, fetchTop10, top10Cards, type Top10Answer } from './top10.ts';

const answer: Top10Answer = {
	week: '2026-10-04',
	region: 'global',
	regions: ['global', 'US'],
	items: [
		{ rank: 1, tmdb_id: '11', kind: 'movie', name: 'UNABOMBER', poster: 'https://image.tmdb.org/t/p/w342/u.jpg', in_library: true, title_id: 7 },
		{ rank: 4, tmdb_id: '12', kind: 'show', name: 'The Gentlemen', season_title: 'The Gentlemen: Season 2', in_library: false, ask: { id: 3, state: 'asked', state_label: 'Asked', mine: true, people: 1 } }
	]
};

describe('top10Cards', () => {
	it('keeps the rank and maps items to poster cards', () => {
		const cards = top10Cards(answer);
		expect(cards.map((c) => c.rank)).toEqual([1, 4]);
		expect(cards[0]).toMatchObject({ key: 'top10-movie-11', name: 'UNABOMBER', titleId: 7, image: 'https://image.tmdb.org/t/p/w342/u.jpg', status: 'in_library', ref: 'tmdb:11' });
		expect(cards[1]).toMatchObject({ kind: 'show', subtitle: 'Season 2', mine: true, titleId: undefined });
	});
});

describe('defaultRegion', () => {
	it('takes the country of the first language that has one', () => {
		expect(defaultRegion(['en', 'en-GB', 'fr-FR'])).toBe('GB');
		expect(defaultRegion(['es-419'])).toBe('global');
		expect(defaultRegion([])).toBe('global');
	});
});

describe('fetchTop10', () => {
	it('falls back to global when the country has no list, and is null when nothing is there', async () => {
		const asked: string[] = [];
		const client = {
			get: async <T>(p: string): Promise<T> => {
				asked.push(p);
				if (p.includes('region=global')) return answer as T;
				throw new ApiError('none', 404);
			}
		};
		expect((await fetchTop10('movie', 'BR', client))?.week).toBe('2026-10-04');
		expect(asked).toEqual(['/top10?kind=movie&region=BR', '/top10?kind=movie&region=global']);
		const none = { get: async () => { throw new ApiError('none', 404); } };
		expect(await fetchTop10('show', 'global', none)).toBeNull();
	});
});
