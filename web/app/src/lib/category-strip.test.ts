import { describe, expect, it } from 'vitest';
import { kin, ranked, seeded, stripTiles } from './category-strip.ts';
import type { BrowseTile } from './search-browse.ts';

const tile = (name: string, kind: 'movie' | 'show' = 'movie', count = 20): BrowseTile => ({ kind, genre: 1, genre_name: name, name, backdrop: `/${name}.jpg`, from: '', category: name.toLowerCase(), count });
const many = (prefix: string, n: number) => Array.from({ length: n }, (_, i) => tile(`${prefix}${i}`));

describe('stripTiles', () => {
	it('takes one from each section in turn and repeats the pattern', () => {
		const s = { genres: many('G', 5), moods: many('M', 5), stories: many('S', 5), worlds: many('W', 5), eras: many('E', 5), world: many('A', 5), collections: many('C', 5) };
		const got = stripTiles(s, 'all', 'x');
		expect(got).toHaveLength(18);
		expect(got.slice(0, 8).map((t) => t.section)).toEqual(['genres', 'moods', 'stories', 'worlds', 'eras', 'world', 'collections', 'genres']);
	});
	it('skips empty sections and keeps the tab kind only', () => {
		const got = stripTiles({ genres: [tile('Drama', 'show'), tile('Comedy')], eras: [tile('80s', 'show')] }, 'shows', 'x');
		expect(got.map((t) => t.name)).toEqual(['Drama', '80s']);
	});
	it('never puts near-duplicates side by side', () => {
		const got = stripTiles({ genres: [tile('Thriller'), tile('Drama')], moods: [tile('Edge-of-your-seat'), tile('Cozy')] }, 'all', 'x');
		for (let i = 1; i < got.length; i++) expect(kin(got[i].name)).not.toBe(kin(got[i - 1].name));
		expect(got.map((t) => t.name).slice(0, 2)).not.toEqual(['Thriller', 'Edge-of-your-seat']);
	});
	it('is the same for one seed and turns over with another', () => {
		const s = { genres: many('G', 12), moods: many('M', 12) };
		expect(stripTiles(s, 'all', 'ana|2026-10-21')).toEqual(stripTiles(s, 'all', 'ana|2026-10-21'));
		const days = new Set(Array.from({ length: 6 }, (_, d) => stripTiles(s, 'all', `ana|day${d}`).map((t) => t.name).join()));
		expect(days.size).toBeGreaterThan(1);
	});
});

describe('ranked', () => {
	it('keeps taste order roughly: the last never jumps to first', () => {
		const r = ranked(many('T', 10), seeded('s'));
		expect(r[0].name).not.toBe('T9');
	});
});
