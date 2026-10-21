import { describe, expect, it } from 'vitest';
import { fromIcon, pageLabel } from './came-from.svelte.ts';

describe('came-from', () => {
	it('names a page from its title', () => expect(pageLabel('Search · HolyArrs')).toBe('Search'));
	it('picks the icon by page', () => {
		expect(fromIcon('/search?q=drake')).toBe('search');
		expect(fromIcon('/explore')).toBe('compass');
		expect(fromIcon('/people/64295')).toBe('people');
		expect(fromIcon('/')).toBe('home');
		expect(fromIcon('/library?kind=movie')).toBe(null);
	});
});
