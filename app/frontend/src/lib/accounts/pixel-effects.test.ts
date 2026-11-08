import { ambient, buttonGrid, burst, busyBand, entrySide, flicker, sweep } from './pixel-trail.ts';
import { describe, expect, it } from 'vitest';

const lit = (e: Float32Array) => e.reduce((n, v) => n + (v > 0 ? 1 : 0), 0);

describe('button pixel effects', () => {
	it('finds the entry side', () => {
		expect(entrySide(300, 50, 2, 25)).toBe('left');
		expect(entrySide(300, 50, 298, 25)).toBe('right');
		expect(entrySide(300, 50, 150, 1)).toBe('top');
		expect(entrySide(300, 50, 150, 49)).toBe('bottom');
	});
	it('sweeps from the entry side across the whole button', () => {
		const g = buttonGrid(312, 52);
		sweep(g, 'left', 0.1);
		expect(g.energy[0]).toBe(1);
		expect(g.energy[g.cols - 1]).toBe(0);
		const h = buttonGrid(312, 52);
		sweep(h, 'right', 0.1);
		expect(h.energy[h.cols - 1]).toBe(1);
		expect(h.energy[0]).toBe(0);
		const end = buttonGrid(312, 52);
		sweep(end, 'left', 0.95);
		expect(end.energy[end.cols - 1]).toBeGreaterThan(0);
	});
	it('keeps the ambient shimmer low and partial', () => {
		const g = buttonGrid(312, 52);
		ambient(g, 1000);
		expect(Math.max(...g.energy)).toBeLessThanOrEqual(0.27);
		expect(lit(g.energy)).toBeGreaterThan(0);
		expect(lit(g.energy)).toBeLessThan(g.energy.length);
	});
	it('bursts as a ring that reaches the far corner', () => {
		const g = buttonGrid(312, 52);
		burst(g, 156, 26, 0.02);
		expect(g.energy[Math.floor(g.cols / 2)]).toBeGreaterThan(0);
		expect(g.energy[0]).toBe(0);
		const f = buttonGrid(312, 52);
		for (let k = 0; k < 1; k += 1 / 21) burst(f, 156, 26, k); // 350 ms at 60 fps.
		expect(f.energy[0]).toBeGreaterThan(0);
		expect(f.energy[f.energy.length - 1]).toBeGreaterThan(0);
	});
	it('loops a busy band and flickers a scatter', () => {
		const g = buttonGrid(312, 52);
		busyBand(g, 0.5);
		expect(lit(g.energy)).toBeGreaterThan(0);
		expect(lit(g.energy)).toBeLessThan(g.energy.length);
		const f = buttonGrid(312, 52);
		flicker(f, 0);
		expect(lit(f.energy)).toBeGreaterThan(0);
	});
});
