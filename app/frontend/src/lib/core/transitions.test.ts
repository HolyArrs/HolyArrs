import { describe, expect, it } from 'vitest';
import { rowGlide, sameRow } from './transitions.ts';

const box = (left: number, top: number, w = 300, h = 200) => ({ left, top, width: w, height: h, right: left + w, bottom: top + h, x: left, y: top }) as DOMRect;

describe('sameRow', () => {
	it('treats a sideways move as the same row', () => {
		expect(sameRow(box(320, 100), box(0, 100))).toBe(true);
		expect(sameRow(box(320, 100), box(0, 104))).toBe(true);
	});
	it('treats a move up or down a row as a new row', () => {
		expect(sameRow(box(0, 320), box(640, 100))).toBe(false);
		expect(sameRow(box(0, 900), box(0, 680))).toBe(false);
	});
});

describe('rowGlide', () => {
	it('fades a card in where it lands when it changes row, so it never crosses its neighbours', () => {
		const a = rowGlide({} as Element, { from: box(0, 320), to: box(640, 100) }, { duration: 300 });
		expect(a.css?.(0, 1)).toContain('opacity: 0');
		expect(a.css?.(1, 0)).toContain('opacity: 1');
		expect(a.css?.(0.5, 0.5)).not.toContain('transform');
	});
	it('does nothing with no time to move', () => {
		const a = rowGlide({} as Element, { from: box(0, 320), to: box(640, 100) }, { duration: 0 });
		expect(a.duration).toBe(0);
	});
});
