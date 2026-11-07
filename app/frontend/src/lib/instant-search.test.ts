import { describe, expect, it } from 'vitest';
import { artistHits, filterSections, libraryByTmdb, movieHits, options, searchPath, showHits, type Section } from './instant-search.ts';
import type { TitleView } from './core/types.ts';

const lib = [{ id: 7, kind: 'movie', name: 'Dune', year: 2021, ids: { tmdb: '438631' }, artwork: ['https://p/1.jpg'] }] as unknown as TitleView[];

describe('instant search', () => {
	it('marks movies the Library has and sends the rest to their details', () => {
		const hits = movieHits(
			[
				{ name: 'Dune', year: 2021, overview: '', original_language: 'en', ids: { tmdb: '438631' } },
				{ name: 'Dune: Part Two', year: 2024, overview: '', original_language: 'en', ids: { tmdb: '693134' } }
			],
			libraryByTmdb(lib)
		);
		expect(hits.map((h) => [h.href, h.inLibrary])).toEqual([
			['/library/7', true],
			['/title/movie/693134', false]
		]);
	});

	it('uses the lookup Library id for shows and falls back to a search without a TMDB id', () => {
		const hits = showHits(
			[
				{ source: 'tmdb', name: 'Fargo', status: 'running', ids: { tmdb: '60622' }, title_id: 160 },
				{ source: 'tvmaze', name: 'Odd', status: 'ended', ids: { tvmaze: '9' } }
			],
			new Map()
		);
		expect(hits.map((h) => h.href)).toEqual(['/library/160', '/search?q=Odd&kind=show']);
	});

	it('matches a Library show by name and year when its ids differ', () => {
		const shows = [{ id: 160, kind: 'show', name: 'Fargo', year: 2014, ids: { tmdb: '9800005' }, artwork: [] }] as unknown as TitleView[];
		const hits = showHits([{ source: 'tmdb', name: 'Fargo', year: 2014, status: 'running', ids: { tmdb: '60622' } }], libraryByTmdb(shows));
		expect(hits[0]).toMatchObject({ href: '/library/160', inLibrary: true });
	});

	it('sends artists not in the Library to a music search', () => {
		expect(artistHits([{ mbid: 'x', name: 'Tin Lanterns', status: '' }])[0].href).toBe('/search?q=Tin%20Lanterns&kind=music');
	});

	it('lists places, hits, More only for full sections, then See all', () => {
		const five = Array.from({ length: 5 }, (_, i) => ({ key: `k${i}`, name: `n${i}`, kindWord: 'Movie', href: '/x', inLibrary: false }));
		const sections: Section[] = [
			{ id: 'library', label: 'Library', hits: five.slice(0, 1), state: 'done', more: '/library' },
			{ id: 'movies', label: 'Movies', hits: five, state: 'done', more: searchPath('du') }
		];
		const o = options(2, sections, 'du');
		expect(o.map((x) => x.kind)).toEqual(['place', 'place', 'hit', 'hit', 'hit', 'hit', 'hit', 'hit', 'more', 'all']);
		expect(o.at(-1)).toEqual({ kind: 'all', href: '/search?q=du' });
	});
});

describe('instant search filter', () => {
	const hit = (key: string, kindWord: string) => ({ key, name: key, kindWord, href: '/', inLibrary: true });
	const sections: Section[] = [
		{ id: 'library', label: 'Library', state: 'done', hits: [hit('a', 'Movie'), hit('b', 'TV show'), hit('c', 'Artist')] },
		{ id: 'movies', label: 'Movies', state: 'done', hits: [hit('m', 'Movie')] },
		{ id: 'shows', label: 'TV shows', state: 'done', hits: [hit('s', 'TV show')] },
		{ id: 'music', label: 'Music', state: 'done', hits: [hit('r', 'Artist')] }
	];
	it('keeps every group for All', () => {
		expect(filterSections(sections, 'all')).toEqual(sections);
	});
	it('keeps one kind: its Library titles and its own group', () => {
		const out = filterSections(sections, 'shows');
		expect(out.map((s) => s.id)).toEqual(['library', 'shows']);
		expect(out[0].hits.map((h) => h.key)).toEqual(['b']);
	});
	it('keeps the Library group, empty, when it has nothing of that kind', () => {
		const out = filterSections([{ ...sections[0], hits: [hit('a', 'Movie')] }, sections[3]], 'music');
		expect(out.map((s) => s.id)).toEqual(['library', 'music']);
		expect(out[0].hits).toEqual([]);
	});
});
