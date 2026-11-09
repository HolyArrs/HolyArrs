import { describe, expect, it } from 'vitest';
import { flipTransform, growPlacement, queue } from './grow.ts';

const view = { width: 1440, height: 900 };
const row = { left: 40, right: 1400 };

describe('queue', () => {
	it('runs at most max jobs at once, all of them in the end', async () => {
		const run = queue(2);
		let live = 0;
		let peak = 0;
		const job = () =>
			run(async () => {
				live++;
				peak = Math.max(peak, live);
				await new Promise((r) => setTimeout(r, 5));
				live--;
				return 1;
			});
		const all = await Promise.all([job(), job(), job(), job(), job()]);
		expect(all).toEqual([1, 1, 1, 1, 1]);
		expect(peak).toBe(2);
	});
});

describe('growPlacement', () => {
	it('grows about 1.5x, centred on the card', () => {
		const art = { left: 500, top: 300, width: 240, height: 135 };
		const g = growPlacement(art, 120, view, row);
		expect(g.width).toBe(360);
		expect(g.mediaHeight).toBe(203);
		expect(g.left + g.width / 2).toBe(620);
		expect(g.top + g.mediaHeight / 2).toBeCloseTo(367.5, 0);
	});

	it('starts exactly on the card (FLIP)', () => {
		const art = { left: 500, top: 300, width: 240, height: 135 };
		const g = growPlacement(art, 120, view, row);
		expect(g.left + g.from.x).toBe(500);
		expect(g.top + g.from.y).toBe(300);
		expect(g.width * g.from.scale).toBeCloseTo(240, 5);
		expect(flipTransform(g.from)).toBe(`translate(${g.from.x.toFixed(2)}px, ${g.from.y.toFixed(2)}px) scale(0.6667)`);
	});

	it('grows inward at the row start', () => {
		const art = { left: 40, top: 300, width: 240, height: 135 };
		const g = growPlacement(art, 120, view, row);
		expect(g.left).toBe(40);
	});

	it('grows inward at the row end', () => {
		const art = { left: 1160, top: 300, width: 240, height: 135 };
		const g = growPlacement(art, 120, view, row);
		expect(g.left + g.width).toBe(1400);
	});

	it('stays inside the window at the bottom and top', () => {
		const low = growPlacement({ left: 500, top: 820, width: 240, height: 135 }, 120, view, row);
		expect(low.top + low.mediaHeight + 120).toBeLessThanOrEqual(892);
		const high = growPlacement({ left: 500, top: -60, width: 240, height: 135 }, 120, view, row);
		expect(high.top).toBe(8);
	});

	it('keeps the size limits and never shrinks below the card', () => {
		expect(growPlacement({ left: 100, top: 300, width: 150, height: 84 }, 0, view).width).toBe(240);
		expect(growPlacement({ left: 100, top: 300, width: 500, height: 281 }, 0, view).width).toBe(560);
		const phone = growPlacement({ left: 16, top: 300, width: 230, height: 129 }, 100, { width: 390, height: 800 });
		expect(phone.width).toBeLessThanOrEqual(374);
		expect(phone.width).toBeGreaterThanOrEqual(230);
		expect(phone.left).toBeGreaterThanOrEqual(8);
	});
});

describe('the grown card at rest', () => {
	it('sits on whole pixels and its start transform lands exactly on the card', () => {
		const art = { left: 100.4, top: 300.6, width: 183.3, height: 103.1 };
		const g = growPlacement(art, 120, { width: 1440, height: 900 });
		for (const v of [g.left, g.top, g.width, g.mediaHeight]) expect(Number.isInteger(v)).toBe(true);
		expect(g.from.scale).toBeCloseTo(art.width / g.width, 6);
		expect(g.left + g.from.x).toBeCloseTo(art.left, 6);
		expect(g.top + g.from.y).toBeCloseTo(art.top, 6);
		expect(g.width * g.from.scale).toBeCloseTo(art.width, 6);
	});
});
