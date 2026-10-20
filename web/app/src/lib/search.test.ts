import { describe, expect, it } from 'vitest';
import { kindOptions, searchAddress } from './search.ts';

describe('search address', () => {
	it('writes both the selected kind and trimmed query from current controls', () => {
		const readonlyLocation: Pick<URL, 'href'> = { href: 'http://holyarrs.test/search?q=old' };
		expect(searchAddress(readonlyLocation, 'show', '  Harbour Lights  ').href).toBe(
			'http://holyarrs.test/search?q=Harbour+Lights&kind=show'
		);
		expect(searchAddress(new URL('http://holyarrs.test/search?kind=music&q=old'), 'movie', ' Dune ').href).toBe(
			'http://holyarrs.test/search?q=Dune'
		);
	});
});

describe('search kinds', () => {
	it('offers Movies, TV shows and Music in that order', () => {
		expect(kindOptions(() => true).map((o) => [o.value, o.label])).toEqual([['movie', 'Movies'], ['show', 'TV shows'], ['music', 'Music']]);
	});
	it('leaves out kinds whose part is off', () => {
		expect(kindOptions((p) => p !== 'music').map((o) => o.value)).toEqual(['movie', 'show']);
		expect(kindOptions((p) => p === 'shows').map((o) => o.value)).toEqual(['show']);
	});
});

import { addRecent, readRecent, readScope, scopeAddress, topMatches, RECENT_MAX } from './search.ts';

describe('search page scope and recents', () => {
	it('reads and writes the scope in the address', () => {
		expect(readScope('people')).toBe('people');
		expect(readScope('nope')).toBe('all');
		expect(readScope(null)).toBe('all');
		expect(scopeAddress({ href: 'http://h.test/search?kind=show' }, 'all', ' dune ').href).toBe('http://h.test/search?q=dune');
		expect(scopeAddress({ href: 'http://h.test/search?q=x' }, 'music', '').href).toBe('http://h.test/search?kind=music');
	});
	it('keeps recent searches unique, newest first and capped', () => {
		expect(addRecent(['Dune', 'Alien'], 'dune')).toEqual(['dune', 'Alien']);
		expect(addRecent(['a'], '  ')).toEqual(['a']);
		const many = Array.from({ length: 20 }, (_, i) => `q${i}`).reduce(addRecent, [] as string[]);
		expect(many).toHaveLength(RECENT_MAX);
		expect(many[0]).toBe('q19');
		expect(readRecent('not json')).toEqual([]);
		expect(readRecent('["a",3,"b"]')).toEqual(['a', 'b']);
	});
	it('takes top matches from each group in turn', () => {
		expect(topMatches([[1, 2, 3], [10], [20, 21]], 5)).toEqual([1, 10, 20, 2, 21]);
		expect(topMatches([[], []])).toEqual([]);
	});
});
