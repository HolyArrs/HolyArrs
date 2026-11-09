import { describes, found, plainPath, understood, type PlainQuery } from './search-plain.ts';
import { describe, expect, it } from 'vitest';

const q = (x: Partial<PlainQuery>): PlainQuery => ({ text: '', plain: false, name: '', genres: [], people: [], words: [], ...x });

describe('plain-words search', () => {
	it('asks only for described searches', () => {
		for (const t of ['that space movie with the robot from 2014', 'scary movies from the 90s', 'funny shows', 'something with dragons about magic', 'nineties sci-fi']) expect(describes(t), t).toBe(true);
		for (const t of ['2012', 'Se7en', 'Dune 2021', 'Blade Runner 2049', 'tmdb:693134', '', '   ', 'Space Jam', 'show me']) expect(describes(t), t).toBe(false);
		expect(describes('movie '.repeat(80))).toBe(false);
	});

	it('says what was understood', () => {
		expect(understood(q({ kind: 'movie', genres: ['Science Fiction'], year_from: 2014, year_to: 2014, words: ['robot'] }))).toBe('Science Fiction movies · 2014 · about robot');
		expect(understood(q({ year_from: 1990, year_to: 1999, genres: ['Horror'] }))).toBe('Horror titles · the 1990s');
		expect(understood(q({ name: 'batman', kind: 'movie', year_from: 2011 }))).toBe('“batman” · movies · 2011 or later');
		expect(understood(q({ kind: 'show', year_to: 1969, people: ['Tom Hanks'] }))).toBe('shows · 1969 or earlier · with Tom Hanks');
		expect(understood(q({ plain: true, name: 'Se7en' }))).toBe('');
		expect(understood(null)).toBe('');
	});

	it('builds the address and knows when there is something to show', () => {
		expect(plainPath('  space & robots ')).toBe('/search?q=space%20%26%20robots');
		expect(found(null)).toBe(false);
		expect(found({ query: q({}), titles: [], more: [] })).toBe(false);
		expect(found({ query: q({ plain: true }), titles: [{} as never], more: [] })).toBe(false);
		expect(found({ query: q({}), titles: [], more: [{} as never] })).toBe(true);
	});
});
