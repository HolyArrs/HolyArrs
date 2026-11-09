import { describe, expect, it, vi } from 'vitest';
import { canViewTransition, navigateWithTransition, pressScale, reducedMotion, safeTransitionName, staggerDelay } from './motion.ts';

describe('motion', () => {
	it('staggers 30ms per item and stops after 8', () => {
		expect(staggerDelay(0)).toBe(0);
		expect(staggerDelay(1)).toBe(30);
		expect(staggerDelay(7)).toBe(210);
		expect(staggerDelay(40)).toBe(210);
		expect(staggerDelay(-2)).toBe(0);
		expect(staggerDelay(Number.NaN)).toBe(0);
		expect(staggerDelay(5, 30, 8, true)).toBe(0);
	});
	it('presses each kind by the spec amount', () => {
		expect(pressScale('button')).toBe(0.97);
		expect(pressScale('round')).toBe(0.92);
		expect(pressScale('card')).toBe(0.98);
		expect(pressScale('round', true)).toBe(1);
	});
	it('reads reduced motion safely', () => {
		expect(reducedMotion(undefined)).toBe(false);
		expect(reducedMotion({ matchMedia: () => ({ matches: true }) })).toBe(true);
		const broken = {
			matchMedia: (): { matches: boolean } => {
				throw Error('no');
			}
		};
		expect(reducedMotion(broken)).toBe(false);
	});
	it('makes transition names valid idents', () => {
		expect(safeTransitionName('title-art-12')).toBe('title-art-12');
		expect(safeTransitionName('12 a/b')).toBe('vt-12-a-b');
	});
	it('only animates page changes when supported and allowed', () => {
		expect(canViewTransition(undefined, false)).toBe(false);
		expect(canViewTransition({}, false)).toBe(false);
		expect(canViewTransition({ startViewTransition: () => undefined }, true)).toBe(false);
		expect(canViewTransition({ startViewTransition: () => undefined }, false)).toBe(true);
	});
	it('skips the transition without support or on same-page changes', async () => {
		const nav = { complete: Promise.resolve(), from: { url: new URL('http://x/a') }, to: { url: new URL('http://x/a?k=1') } };
		expect(navigateWithTransition(nav, {})).toBeUndefined();
		const start = vi.fn((cb: () => Promise<void> | void) => void cb());
		expect(navigateWithTransition({ ...nav, from: { url: new URL('http://x/settings') }, to: { url: new URL('http://x/settings/api') } }, { startViewTransition: start })).toBeUndefined();
		expect(navigateWithTransition(nav, { startViewTransition: start })).toBeUndefined();
		const p = navigateWithTransition({ ...nav, to: { url: new URL('http://x/b') } }, { startViewTransition: start });
		expect(p).toBeInstanceOf(Promise);
		await p;
		expect(start).toHaveBeenCalledTimes(1);
	});
});

import { containBox } from './motion.ts';
describe('theater geometry', () => {
	it('fits a 16:9 picture into a wider window, centred', () => {
		expect(containBox(16 / 9, 2000, 900)).toEqual({ left: 200, top: 0, width: 1600, height: 900 });
		expect(containBox(0, 1600, 900).width).toBe(1600);
	});
});

import { cubicBezier, flipFrames } from './motion.ts';
describe('theater flip frames', () => {
	it('evaluates a cubic-bezier like CSS', () => {
		const lin = cubicBezier(0, 0, 1, 1);
		expect(lin(0.5)).toBeCloseTo(0.5, 3);
		expect(cubicBezier(0.22, 1, 0.36, 1)(0.5)).toBeGreaterThan(0.9);
	});
	it('starts on the small rect and ends on the window, the picture never stretched', () => {
		const f = flipFrames({ left: 100, top: 50, width: 320, height: 180 }, 16 / 9, 1600, 900, false, 10);
		expect(f.outer[0]).toBe('translate(100px, 50px) scale(0.2, 0.2)');
		expect(f.outer[10]).toBe('translate(0px, 0px) scale(1, 1)');
		expect(f.inner[10]).toBe('translate(0px, 0px) scale(1, 1)');
		for (const t of f.inner) expect(t).toMatch(/scale\(1, 1\)$/);
		const back = flipFrames({ left: 100, top: 50, width: 320, height: 180 }, 16 / 9, 1600, 900, true, 10);
		expect(back.outer[0]).toBe(f.outer[10]);
		expect(back.outer[10]).toBe(f.outer[0]);
	});
	it('keeps one uniform picture scale when the rect is wider than the picture', () => {
		const f = flipFrames({ left: 0, top: 0, width: 436, height: 200 }, 16 / 9, 1600, 900, false, 8);
		for (let i = 0; i <= 8; i++) {
			const [ox, oy] = f.outer[i].match(/scale\(([\d.]+), ([\d.]+)\)/)!.slice(1).map(Number);
			const [ix, iy] = f.inner[i].match(/scale\(([\d.]+), ([\d.]+)\)/)!.slice(1).map(Number);
			expect(ox * ix).toBeCloseTo(oy * iy, 3);
		}
	});
});
