import { describe, expect, it } from 'vitest';
import { facts, readInfo } from './title-info.ts';

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
	});
});
