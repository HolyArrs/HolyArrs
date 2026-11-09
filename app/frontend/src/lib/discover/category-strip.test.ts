import { fromStrip, ranked, seeded, shownBatcher, stripKey, stripPath, stripRowNames } from './category-strip.ts';
import type { BrowseTile } from './search-browse.ts';
import { describe, expect, it, vi } from 'vitest';

const tile = (name: string, kind: 'movie' | 'show' = 'movie', count = 20): BrowseTile => ({ kind, genre: 1, genre_name: name, name, backdrop: `/${name}.jpg`, from: '', category: name.toLowerCase(), count });
const many = (prefix: string, n: number) => Array.from({ length: n }, (_, i) => tile(`${prefix}${i}`));

describe('ranked', () => {
	it('keeps taste order roughly: the last never jumps to first', () => {
		const r = ranked(many('T', 10), seeded('s'));
		expect(r[0].name).not.toBe('T9');
	});
});

describe('strip helpers', () => {
	it('passes the rows below and the time zone', () => {
		expect(stripPath('shows', ['Popular Comedy', 'Because you asked'], 'Europe/Paris')).toBe('/explore/strip?type=shows&row=Popular+Comedy&row=Because+you+asked&tz=Europe%2FParis');
		expect(stripPath('all', [])).toBe('/explore/strip?type=all');
	});
	it('names each row once, trimmed and bounded', () => {
		const rows = [{ title: ' Popular Comedy ' }, { title: 'Popular Comedy' }, { title: '' }, {}, { title: 'x'.repeat(81) }];
		expect(stripRowNames(rows)).toEqual(['Popular Comedy']);
		expect(stripRowNames(Array.from({ length: 50 }, (_, i) => ({ title: `Row ${i}` })))).toHaveLength(40);
	});
	it('keys tiles by category, else the genre name', () => {
		expect(stripKey({ category: 'heists', name: 'Heists' })).toBe('heists');
		expect(stripKey({ category: '', name: 'Sci-Fi' })).toBe('sci-fi');
		const [g] = fromStrip([{ ...tile('Drama'), category: '', section: 'genres' }]);
		expect('category' in g).toBe(false);
		expect(g.section).toBe('genres');
	});
	it('sends shown keys once, batched after a quiet spell', () => {
		vi.useFakeTimers();
		const sent: string[][] = [];
		const b = shownBatcher((k) => sent.push(k), 1000);
		b.add(['a', 'b']);
		vi.advanceTimersByTime(500);
		b.add(['b', 'c']);
		vi.advanceTimersByTime(999);
		expect(sent).toEqual([]);
		vi.advanceTimersByTime(1);
		expect(sent).toEqual([['a', 'b', 'c']]);
		b.flush();
		expect(sent).toHaveLength(1);
		vi.useRealTimers();
	});
});
