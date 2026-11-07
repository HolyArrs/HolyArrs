import { describe, expect, it } from 'vitest';
import { mountNow, nextLimit, ROW_FIRST, ROW_STEP, stableRows } from './row-window.ts';

describe('row windowing', () => {
	it('mounts a row within 1.5 screens below the viewport', () => {
		expect(mountNow(0, 900)).toBe(true);
		expect(mountNow(2249, 900)).toBe(true);
		expect(mountNow(2250, 900)).toBe(false);
	});

	it('adds a dozen cards once the row scrolls near its rendered end', () => {
		const at = (scrollLeft: number) => ({ scrollLeft, clientWidth: 1000, scrollWidth: 3000 });
		expect(nextLimit(ROW_FIRST, 20, at(0))).toBe(ROW_FIRST);
		expect(nextLimit(ROW_FIRST, 20, at(1500))).toBe(20);
		expect(nextLimit(ROW_FIRST, 20, at(1400))).toBe(ROW_FIRST);
		expect(nextLimit(ROW_FIRST, 40, at(1500))).toBe(ROW_FIRST + ROW_STEP);
		expect(nextLimit(20, 20, at(2000))).toBe(20);
	});

	it('keeps rows where they were first shown; new rows go after', () => {
		const id = (r: { id: string }) => r.id;
		const first = stableRows([], [{ id: 'a' }, { id: 'b' }, { id: 'c' }], id);
		expect(first.order).toEqual(['a', 'b', 'c']);
		const later = stableRows(first.order, [{ id: 'new' }, { id: 'c', n: 2 }, { id: 'a' }], id);
		expect(later.rows).toEqual([{ id: 'a' }, { id: 'c', n: 2 }, { id: 'new' }]);
		expect(later.order).toEqual(['a', 'c', 'new']);
	});
});
