import { describe, expect, it } from 'vitest';
import { searchAddress } from './search.ts';

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
