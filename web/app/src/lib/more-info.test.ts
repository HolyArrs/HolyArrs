import { describe, expect, it } from 'vitest';
import { bigRows, facts, firstSeason, parseTitleParam, readCardOpens, readEpisodes, readInfo, seasonLabel, titleParam, withTitle } from './more-info.ts';

describe('the panel in the URL', () => {
	it('reads movie and show refs and refuses anything else', () => {
		expect(parseTitleParam('movie:123')).toEqual({ kind: 'movie', tmdb: '123' });
		expect(parseTitleParam(' show:9 ')).toEqual({ kind: 'show', tmdb: '9' });
		for (const bad of [null, undefined, '', 'artist:1', 'movie:', 'movie:0', 'movie:12a', 'movie:-1', 'movie:01', 'movie:12345678901']) expect(parseTitleParam(bad)).toBeNull();
	});
	it('writes the ref and keeps other parameters', () => {
		expect(titleParam({ kind: 'show', tmdb: '7' })).toBe('show:7');
		const open = withTitle('https://h.test/explore?type=movies', { kind: 'movie', tmdb: '5' });
		expect(open.searchParams.get('title')).toBe('movie:5');
		expect(open.search).toBe('?type=movies&title=movie:5');
		expect(open.searchParams.get('type')).toBe('movies');
		const closed = withTitle(open, null);
		expect(closed.search).toBe('?type=movies');
		expect(parseTitleParam(open.searchParams.get('title'))).toEqual({ kind: 'movie', tmdb: '5' });
	});
	it('card clicks open the title page unless the device chose the panel', () => {
		expect(readCardOpens(null)).toBe('page');
		expect(readCardOpens('odd')).toBe('page');
		expect(readCardOpens('panel')).toBe('panel');
	});
});

describe('the details answer', () => {
	const raw = {
		name: 'Seven', year: 2020, rating: 'R', runtime_minutes: 99, overview: 'Words.', genres: ['Drama', 3],
		people: [...Array.from({ length: 10 }, (_, i) => ({ name: `A${i}`, role: 'actor' })), { name: 'Dee', role: 'director' }],
		streaming: [{ id: 8, name: 'Netflix', logo: 'n.png' }, { id: 0, name: 'x' }],
		similar: [{ tmdb_id: '2', kind: 'movie', name: 'Two', in_library: false }, { kind: 'movie', name: 'no id' }],
		collection: { name: 'Set', parts: [{ tmdb_id: '7', kind: 'movie', name: 'Seven' }, { tmdb_id: '8', kind: 'movie', name: 'Eight' }] },
		in_library: true, title_id: 4
	};
	it('reads people, services, similar, collection and the Library', () => {
		const d = readInfo('movie', '7', raw);
		expect(d.cast).toHaveLength(8);
		expect(d.crew).toEqual([{ name: 'Dee', role: 'director', character: undefined, profile: undefined }]);
		expect(d.genres).toEqual(['Drama']);
		expect(d.streaming).toEqual([{ id: 8, name: 'Netflix', logo: 'n.png' }]);
		expect(d.similar.map((s) => s.tmdb_id)).toEqual(['2']);
		expect(d.collection?.parts).toHaveLength(2);
		expect(d.titleId).toBe(4);
		expect(d.head).toMatchObject({ tmdb_id: '7', kind: 'movie', in_library: true, title_id: 4 });
		expect(facts(d)).toEqual(['Movie', '1h 39m', '2020', 'R']);
	});
	it('tolerates an empty answer', () => {
		const d = readInfo('show', '1', {});
		expect(d).toMatchObject({ name: '', cast: [], seasons: [], streaming: [], similar: [], collection: null, titleId: undefined });
		expect(facts(d)).toEqual([]);
	});
	it('counts real seasons and starts on season 1', () => {
		const d = readInfo('show', '1', { year: 2021, seasons: [{ number: 0, episode_count: 3 }, { number: 1, episode_count: 8 }, { number: 2, episode_count: 0 }, { number: 3, episode_count: 6 }] });
		expect(d.seasons.map((s) => s.number)).toEqual([0, 1, 3]);
		expect(facts(d)).toEqual(['Show', '2 seasons', '2021']);
		expect(firstSeason(d.seasons)).toBe(1);
		expect(firstSeason([{ number: 0, episodes: 1 }, { number: 4, episodes: 2 }])).toBe(4);
		expect(firstSeason([])).toBeNull();
		expect(seasonLabel(0)).toBe('Specials');
	});
	it('reads episodes', () => {
		expect(readEpisodes({ episodes: [{ number: 1, name: 'Pilot', runtime: 50, air_day: '2021-01-01' }, { number: 0 }] })).toEqual([
			{ number: 1, name: 'Pilot', overview: undefined, runtime: 50, still: undefined, airDay: '2021-01-01' }
		]);
	});
});

describe('big 4 shelves', () => {
	it('marks the first personal row and Best new releases', () => {
		const rows = [{ id: 'a', type: 'just_arrived' }, { id: 'b', type: 'top_picks' }, { id: 'c', type: 'trending' }, { id: 'd', type: 'genre' }, { id: 'e', type: 'best' }];
		expect([...bigRows(rows)]).toEqual(['b', 'e']);
	});
	it('falls back to the first personal row of another kind', () => {
		expect([...bigRows([{ id: 'x', type: 'trending' }, { id: 'y', type: 'because_have' }, { id: 'z', type: 'genre' }])]).toEqual(['y']);
	});
	it('never puts two big rows next to each other', () => {
		expect([...bigRows([{ id: 'p', type: 'top_picks' }, { id: 'q', type: 'best' }, { id: 'r', type: 'trending' }])]).toEqual(['p']);
		expect([...bigRows([{ id: 'r', type: 'trending' }])]).toEqual([]);
	});
});
