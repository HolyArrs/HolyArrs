import { describe, expect, it } from 'vitest';
import { canReturn, putOffsets, readOffsets } from './explore-return.ts';

// A tiny stand-in for the page: rows with an id and a scrolling element.
function page(rows: Record<string, number>) {
	const els = Object.entries(rows).map(([id, left]) => {
		const row = { scrollLeft: left };
		return { dataset: { rowId: id }, querySelector: () => row, row };
	});
	return { root: { querySelectorAll: () => els } as unknown as ParentNode, els };
}

describe('back to Explore', () => {
	it('keeps each scrolled row by id', () => {
		expect(readOffsets(page({ a: 420, b: 0 }).root)).toEqual({ a: 420 });
	});
	it('puts the rows back, skipping rows not on the page', () => {
		const p = page({ a: 0, b: 0 });
		putOffsets(p.root, { b: 300, gone: 10 });
		expect(p.els.map((e) => e.row.scrollLeft)).toEqual([0, 300]);
	});
	it('waits until the page is tall enough, unless nothing more loads', () => {
		expect(canReturn(1000, 2000, 800, true)).toBe(false);
		expect(canReturn(3000, 2000, 800, true)).toBe(true);
		expect(canReturn(1000, 2000, 800, false)).toBe(true);
	});
});
