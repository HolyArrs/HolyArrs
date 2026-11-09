import { blockMask, blockOrder, buttonGrid, columnsForWidth, decay, siteCell, stampPath, FLOOR } from './pixel-trail.ts';
import { describe, expect, it } from 'vitest';

describe('pixelTrail', () => {
	it('uses the site grid and symbol pixel', () => {
		expect(columnsForWidth(1920)).toBe(36);
		expect(columnsForWidth(1000)).toBe(30);
		expect(columnsForWidth(800)).toBe(22);
		expect(columnsForWidth(390)).toBe(12);
		expect(siteCell(1440)).toBe(40);
	});

	it('sizes a button grid in whole columns', () => {
		const g = buttonGrid(384, 50);
		expect(g.cols).toBe(15);
		expect(g.cell * g.cols).toBeCloseTo(384);
		expect(g.rows).toBe(2);
	});

	it('sprays the cell under the pointer and 0.4 on its neighbours', () => {
		const g = buttonGrid(260, 78);
		stampPath(g, undefined, g.cell * 1.5, g.cell * 1.5);
		expect(g.energy[1 * g.cols + 1]).toBe(1);
		expect(g.energy[1 * g.cols + 0]).toBeCloseTo(0.4);
		expect(g.energy[0 * g.cols + 1]).toBeCloseTo(0.4);
		expect(g.energy[0]).toBe(0);
	});

	it('fills the path between moves', () => {
		const g = buttonGrid(260, 26);
		stampPath(g, { x: 5, y: 10 }, 255, 10);
		for (let c = 0; c < g.cols; c++) expect(g.energy[c]).toBe(1);
	});

	it('decays with the 0.3 s constant and goes idle', () => {
		const g = buttonGrid(52, 26);
		g.energy[0] = 1;
		decay(g, 0.3);
		expect(g.energy[0]).toBeCloseTo(Math.exp(-1));
		let on = true;
		for (let i = 0; i < 200 && on; i++) on = decay(g, 0.05);
		expect(on).toBe(false);
		expect(g.energy.every((v) => v === 0 || v >= FLOOR)).toBe(true);
	});

	it('block masks reveal in a stable order', () => {
		const o = blockOrder(20, 3);
		expect(blockOrder(20, 3)).toEqual(o);
		expect(blockMask(5, 4, o, 0)).not.toContain('rect');
		const all = decodeURIComponent(blockMask(5, 4, o, 1.01));
		expect(all.match(/<rect/g)).toHaveLength(20);
	});
});
