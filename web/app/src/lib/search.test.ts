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
