import { describe, expect, it } from 'vitest';
import { shelfEntries } from './shelf.ts';

describe('shelfEntries', () => {
	const on = () => true;
	it('lists Movies, TV shows, Music and then Stats for people who manage the Library', () => {
		expect(shelfEntries({ on, kid: false, manage: true }).map((e) => e.key)).toEqual(['movie', 'show', 'artist', 'stats']);
	});
	it('leaves Stats out for people who do not manage the Library', () => {
		expect(shelfEntries({ on, kid: false, manage: false }).map((e) => e.key)).toEqual(['movie', 'show', 'artist']);
	});
	it('drops parts that are off and Music for a Kid', () => {
		expect(shelfEntries({ on: (p) => p !== 'shows', kid: true, manage: false }).map((e) => e.key)).toEqual(['movie']);
	});
	it('links Stats to the stats page', () => {
		expect(shelfEntries({ on, kid: false, manage: true }).at(-1)?.href).toBe('/library/stats');
	});
});
