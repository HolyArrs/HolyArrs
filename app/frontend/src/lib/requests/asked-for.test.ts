import { askedFor } from './asked-for.ts';
import { describe, expect, it } from 'vitest';
import type { Ask } from '../core/types.ts';

const ask = (o: Partial<Ask>) => ({ kind: 'movie', ...o }) as Ask;

describe('askedFor', () => {
	it('says nothing more for a movie', () => {
		expect(askedFor(ask({ kind: 'movie' }))).toBe('');
	});
	it('names the seasons of a show, a run as a range', () => {
		expect(askedFor(ask({ kind: 'show', seasons: [] }))).toBe('All seasons');
		expect(askedFor(ask({ kind: 'show', seasons: [2] }))).toBe('Season 2');
		expect(askedFor(ask({ kind: 'show', seasons: [1, 2] }))).toBe('Seasons 1–2');
		expect(askedFor(ask({ kind: 'show', seasons: [3, 1, 2] }))).toBe('Seasons 1–3');
		expect(askedFor(ask({ kind: 'show', seasons: [1, 3] }))).toBe('Seasons 1, 3');
		expect(askedFor(ask({ kind: 'show', seasons: [1, 2, 4] }))).toBe('Seasons 1–2, 4');
	});
	it('says Specials for season 0', () => {
		expect(askedFor(ask({ kind: 'show', seasons: [0] }))).toBe('Specials');
		expect(askedFor(ask({ kind: 'show', seasons: [0, 1] }))).toBe('Specials, Season 1');
	});
	it('counts albums, or names a single one', () => {
		expect(askedFor(ask({ kind: 'artist', albums: [] }))).toBe('All albums');
		expect(askedFor(ask({ kind: 'artist', albums: [{ name: 'Currents' }] as Ask['albums'] }))).toBe('Currents');
		expect(askedFor(ask({ kind: 'artist', albums: [{ name: 'A' }, { name: 'B' }, { name: 'C' }] as Ask['albums'] }))).toBe('3 albums');
	});
	it('puts the total in when known', () => {
		expect(askedFor(ask({ kind: 'artist', albums: [] }), { albums: 9 })).toBe('All 9 albums');
		expect(askedFor(ask({ kind: 'show', seasons: [] }), { seasons: 4 })).toBe('All 4 seasons');
	});
});
