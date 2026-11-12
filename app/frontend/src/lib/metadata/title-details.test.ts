import { describe, expect, it } from 'vitest';
import { ApiError } from '../core/api.ts';
import { detailPath, fourKOffer, fromMetadata, loadDetails, metaLine } from './title-details.ts';

describe('title details', () => {
	it('builds the detail path', () => {
		expect(detailPath('movie', '438631')).toBe('/title/movie/438631');
	});

	it('reads a metadata answer and skips what is missing', () => {
		const d = fromMetadata('movie', '1', {
			title: 'Dune',
			year: 2021,
			genres: [{ name: 'Science Fiction' }, 'Drama'],
			vote_average: 7.84,
			vote_count: 900,
			cast: [{ name: 'Zendaya', character: 'Chani' }, { character: 'nobody' }],
			similar: [{ id: 2, title: 'Arrival', year: 2016 }, { name: 'no id' }]
		});
		expect(d.name).toBe('Dune');
		expect(d.genres).toEqual(['Science Fiction', 'Drama']);
		expect(d.rating).toBe('7.8/10');
		expect(fromMetadata('movie', '1', { title: 'Few', vote_average: 9.1, vote_count: 24 }).rating).toBeUndefined();
		expect(d.cast).toEqual([{ name: 'Zendaya', role: 'Chani', image: undefined, id: undefined }]);
		expect(d.similar).toEqual([{ kind: 'movie', tmdb: '2', name: 'Arrival', year: 2016, poster: undefined }]);
	});

	it('falls back to the lookup when the metadata endpoint is missing', async () => {
		const get = async <T,>(path: string): Promise<T> => {
			if (path.startsWith('/metadata/')) throw new ApiError('not found', 404);
			expect(path).toBe('/titles/lookup?term=tmdb%3A438631');
			return { movies: [{ name: 'Dune', year: 2021, overview: 'Sand.', original_language: 'en', ids: { tmdb: '438631' }, never_add: false, title_id: 7 }] } as T;
		};
		const d = await loadDetails(get, 'movie', '438631');
		expect(d?.name).toBe('Dune');
		expect(d?.titleId).toBe(7);
		expect(d?.movie?.ids.tmdb).toBe('438631');
	});

	it('never takes another title from the lookup when the id does not match', async () => {
		const get = async <T,>(path: string): Promise<T> => {
			if (path.startsWith('/metadata/')) throw new ApiError('not found', 404);
			return (path.startsWith('/titles/') ? { movies: [{ name: 'The Cabin in the Woods', year: 2011, ids: { tmdb: '22970' }, title_id: 3 }] } : { shows: [{ source: 'tmdb', name: 'Other', ids: { tmdb: '9' }, title_id: 4 }] }) as T;
		};
		expect(await loadDetails(get, 'movie', '1')).toBeNull();
		expect(await loadDetails(get, 'show', '2')).toBeNull();
	});

	it('keeps the lookup Library facts over the metadata answer', async () => {
		const get = async <T,>(path: string): Promise<T> =>
			(path.startsWith('/metadata/') ? { name: 'Fargo', backdrop: 'https://x/b.jpg', genres: ['Crime'] } : { shows: [{ source: 'tmdb', name: 'Fargo', status: 'running', ids: { tmdb: '60622' }, title_id: 160 }] }) as T;
		const d = await loadDetails(get, 'show', '60622');
		expect(d?.backdrop).toBe('https://x/b.jpg');
		expect(d?.titleId).toBe(160);
		expect(d?.show?.name).toBe('Fargo');
	});

	it('asks the metadata and the lookup at once and hands the metadata answer over first', async () => {
		const asked: string[] = [];
		let answerLookup: (v: unknown) => void = () => {};
		const get = <T,>(path: string): Promise<T> => {
			asked.push(path);
			if (path.startsWith('/metadata/')) return Promise.resolve({ name: 'Dune', backdrop: 'https://x/b.jpg', genres: [] } as T);
			return new Promise<T>((r) => (answerLookup = r as (v: unknown) => void));
		};
		const early: string[] = [];
		const d = loadDetails(get, 'movie', '438631', undefined, (m) => early.push(m.name));
		expect(asked).toEqual(['/metadata/movie/438631', '/titles/lookup?term=tmdb%3A438631']);
		await new Promise((r) => setTimeout(r, 0));
		expect(early).toEqual(['Dune']);
		answerLookup({ movies: [{ name: 'Dune', ids: { tmdb: '438631' }, never_add: false, title_id: 7 }] });
		expect((await d)?.titleId).toBe(7);
	});

	it('shows the metadata answer when the lookup fails, and fails when both do', async () => {
		const get = async <T,>(path: string): Promise<T> => {
			if (path.startsWith('/metadata/')) return { name: 'Dune', genres: [] } as T;
			throw new ApiError('busy', 503);
		};
		expect((await loadDetails(get, 'movie', '1'))?.name).toBe('Dune');
		const none = async <T,>(path: string): Promise<T> => {
			throw new ApiError(path.startsWith('/metadata/') ? 'not found' : 'busy', path.startsWith('/metadata/') ? 404 : 503);
		};
		await expect(loadDetails(none, 'movie', '1')).rejects.toThrow('busy');
	});

	it('says year, kind, runtime and genres', () => {
		expect(metaLine({ kind: 'movie', year: 2021, runtime: 155, genres: ['Drama'] })).toEqual(['2021', 'Movie', '2h 35m', 'Drama']);
		expect(metaLine({ kind: 'show', genres: [], network: 'FX' })).toEqual(['TV show', 'FX']);
	});

	it('keeps the lookup 4K ask facts over the metadata answer', async () => {
		const ask4k = { id: 9, state: 'asked' as const, state_label: 'Asked', mine: true, people: 1 };
		const get = async <T,>(path: string): Promise<T> =>
			(path.startsWith('/metadata/') ? { name: 'Dune', genres: [] } : { movies: [{ name: 'Dune', ids: { tmdb: '438631' }, never_add: false, title_id: 7, ask_4k: ask4k, has_4k: false }] }) as T;
		const d = await loadDetails(get, 'movie', '438631');
		expect(d?.ask4k).toEqual(ask4k);
		expect(d?.has4k).toBe(false);
	});

	it('offers a separate 4K ask after my plain ask or on a movie in the Library without 4K', () => {
		const mine = { id: 1, state: 'asked' as const, state_label: 'Asked', mine: true, people: 1 };
		const theirs = { ...mine, mine: false };
		expect(fourKOffer({ kind: 'movie' })).toBeNull();
		expect(fourKOffer({ kind: 'movie', ask: theirs })).toBeNull();
		expect(fourKOffer({ kind: 'movie', ask: mine })).toBe('ask');
		expect(fourKOffer({ kind: 'movie', titleId: 7 })).toBe('ask');
		expect(fourKOffer({ kind: 'movie', titleId: 7, ask4k: theirs })).toBe('ask');
		expect(fourKOffer({ kind: 'movie', titleId: 7, ask4k: mine })).toBe('asked');
		expect(fourKOffer({ kind: 'movie', titleId: 7, has4k: true })).toBeNull();
		expect(fourKOffer({ kind: 'movie', titleId: 7, neverAdd: true })).toBeNull();
		expect(fourKOffer({ kind: 'show', titleId: 7, ask: mine })).toBeNull();
	});
});
