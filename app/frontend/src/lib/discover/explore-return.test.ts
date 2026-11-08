import { anchorDelta, canReturn, putOffsets, readAnchor, readOffsets } from './explore-return.ts';
import { describe, expect, it } from 'vitest';

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
	it('keeps the first row on screen as the spot, and scrolls it back to the same place', () => {
		const at = (tops: Record<string, number>) => ({ querySelectorAll: () => Object.entries(tops).map(([id, top]) => ({ dataset: { rowId: id }, getBoundingClientRect: () => ({ top, bottom: top + 300 }) })) }) as unknown as ParentNode;
		expect(readAnchor(at({ a: -600, b: -100, c: 220 }), 60)).toEqual({ id: 'b', top: -100 });
		expect(anchorDelta(at({ a: -600, b: 250 }), { id: 'b', top: -100 })).toBe(350);
		expect(anchorDelta(at({ a: 0 }), { id: 'b', top: -100 })).toBeNull();
		expect(anchorDelta(at({ a: 0 }), undefined)).toBeNull();
		// A display: contents wrapper has no box, so its first child is measured.
		const wrap = { querySelectorAll: () => [{ dataset: { rowId: 'w' }, getBoundingClientRect: () => ({ top: 0, bottom: 0 }), firstElementChild: { getBoundingClientRect: () => ({ top: 140, bottom: 400 }) } }] } as unknown as ParentNode;
		expect(readAnchor(wrap)).toEqual({ id: 'w', top: 140 });
	});
});
