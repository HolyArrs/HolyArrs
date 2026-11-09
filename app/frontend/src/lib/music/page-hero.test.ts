import { heroArtists, heroLine } from './page-hero.ts';
import { describe, expect, it } from 'vitest';

describe('Browse page hero words', () => {
	it('says what the page sounds like, then its leading artists', () => {
		expect(heroLine('genre:rnb', 'R&B', ['SZA', 'Frank Ocean', 'Daniel Caesar'])).toBe('Smooth vocals and slow grooves: SZA, Frank Ocean, Daniel Caesar and more');
		expect(heroLine('decade:1990s', '90s', [])).toBe('The albums that defined the 90s');
	});
	it('takes each artist once, in page order', () => {
		const card = (artist: string) => ({ key: artist + Math.random(), kind: 'album', name: 'x', artist }) as never;
		expect(heroArtists([card('SZA')], [{ id: 'r', items: [card('SZA'), card('Usher'), card('Brent'), card('Kehlani')] } as never])).toEqual(['SZA', 'Usher', 'Brent']);
	});
});
