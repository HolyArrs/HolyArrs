import { describe, expect, it } from 'vitest';

import { addRecent, rankTop, readRecent, readScope, scopeAddress, topMatches, RECENT_MAX } from './search-address.ts';

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
	it('puts exact-name hits first in Top matches', () => {
		const n = (name: string) => ({ name });
		expect(rankTop([[n('Vera Drake'), n('Drake & Josh')], [n('Drake')]], ' drake ', 3).map((c) => c.name)).toEqual(['Drake', 'Vera Drake', 'Drake & Josh']);
		expect(rankTop([[n('Drake')], [n('Drake'), n('Drake'), n('X')]], 'drake', 4).map((c) => c.name)).toEqual(['Drake', 'Drake', 'Drake', 'X']);
		expect(rankTop([[n('A')], [n('B')]], '', 6).map((c) => c.name)).toEqual(['A', 'B']);
	});
});

import { libraryNote } from './search-address.ts';

describe('libraryNote', () => {
	it('says what a Library title is doing', () => {
		expect(libraryNote({ kind: 'show', show: { seasons: [{ number: 0 }, { number: 1 }, { number: 2 }, { number: 3 }, { number: 4 }], missing: 0 } })).toBe('4 seasons · up to date');
		expect(libraryNote({ kind: 'show', show: { seasons: [{ number: 1 }], missing: 2 } })).toBe('Missing 2 episodes');
		expect(libraryNote({ kind: 'movie', slots: [{ state: 'downloading' }] }, 'In your Library')).toBe('Downloading');
		expect(libraryNote({ kind: 'movie', slots: [] }, 'In your Library')).toBe('In your Library');
		expect(libraryNote({ kind: 'artist', artist: { albums: 1, missing: 0 } })).toBe('1 album · up to date');
	});
});
