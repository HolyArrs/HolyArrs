import { describe, expect, it, vi } from 'vitest';
import { lineTops, RowHeights } from './library-view.ts';

describe('lineTops', () => {
	it('uses measured heights first, else the guess', () => {
		const heights = new Map([['b', 30]]);
		expect(lineTops([{ key: 'a' }, { key: 'b' }, { key: 'c' }], heights, () => 10)).toEqual([0, 10, 40, 50]);
	});

	it('asks for one guess per group, however long the list', () => {
		const guess = vi.fn((l: { key: string; kind: string }) => (l.kind === 'movie' ? 54 : 60));
		const lines = Array.from({ length: 5000 }, (_, i) => ({ key: `r${i}`, kind: i % 2 ? 'movie' : 'show' }));
		const tops = lineTops(lines, new Map(), guess, (l) => l.kind);
		expect(tops).toHaveLength(5001);
		expect(tops[2]).toBe(114);
		expect(guess).toHaveBeenCalledTimes(2);
	});
});

describe('RowHeights', () => {
	it('tells once after a burst of changes, not once per row', async () => {
		const changed = vi.fn();
		const h = new RowHeights(changed);
		for (let i = 0; i < 30; i++) h.set(`r${i}`, 50 + i);
		h.delete('r0');
		expect(changed).not.toHaveBeenCalled();
		await Promise.resolve();
		expect(changed).toHaveBeenCalledTimes(1);
		expect(h.get('r29')).toBe(79);
		h.clear();
		await Promise.resolve();
		expect(changed).toHaveBeenCalledTimes(2);
	});

	it('stays quiet when nothing was removed', async () => {
		const changed = vi.fn();
		const h = new RowHeights(changed);
		h.delete('none');
		await Promise.resolve();
		expect(changed).not.toHaveBeenCalled();
	});
});
