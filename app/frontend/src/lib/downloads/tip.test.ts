import { place } from './tip.ts';
import { beforeAll, describe, expect, it, vi } from 'vitest';

const box = (left: number, top: number, width: number, height: number) => ({ left, top, width, height, right: left + width, bottom: top + height }) as DOMRect;

function tipAt(w: number, h: number) {
	const style: Record<string, string> = {};
	return { getBoundingClientRect: () => ({ width: w, height: h }), style } as unknown as HTMLElement;
}

describe('place', () => {
	beforeAll(() => {
		vi.stubGlobal('document', { documentElement: { clientWidth: 1000, clientHeight: 800 } });
	});

	it('puts the tip below the target, centred', () => {
		const t = tipAt(100, 24);
		place(t, box(400, 100, 40, 30), 'below');
		expect(t.style.left).toBe('370px');
		expect(t.style.top).toBe('136px');
	});
	it('stays inside the screen at the right edge and flips above at the bottom', () => {
		const t = tipAt(120, 24);
		place(t, box(980, 790, 20, 10), 'below');
		expect(parseInt(t.style.left)).toBeLessThanOrEqual(1000 - 120 - 8);
		expect(parseInt(t.style.top)).toBeLessThan(790);
	});
	it('goes to the right of a rail icon, or the left when there is no room', () => {
		const t = tipAt(100, 24);
		place(t, box(10, 200, 40, 36), 'right');
		expect(t.style.left).toBe('58px');
		place(t, box(950, 200, 40, 36), 'right');
		expect(parseInt(t.style.left)).toBeLessThan(950);
	});
});
