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
		expect(navigateWithTransition(nav, { startViewTransition: start })).toBeUndefined();
		const p = navigateWithTransition({ ...nav, to: { url: new URL('http://x/b') } }, { startViewTransition: start });
		expect(p).toBeInstanceOf(Promise);
		await p;
		expect(start).toHaveBeenCalledTimes(1);
	});
});

import { clipFrom, containBox, coverFrom } from './motion.ts';
describe('theater geometry', () => {
	it('fits a 16:9 picture into a wider window, centred', () => {
		expect(containBox(16 / 9, 2000, 900)).toEqual({ left: 200, top: 0, width: 1600, height: 900 });
		expect(containBox(0, 1600, 900).width).toBe(1600);
	});
	it('covers the small rect with one uniform scale', () => {
		// 1600x900 window, a 320x180 card at (100, 50): a fifth of the size, centred on the card.
		expect(coverFrom({ left: 100, top: 50, width: 320, height: 180 }, 16 / 9, 1600, 900)).toBe('translate(-540px, -310px) scale(0.2)');
		// A taller rect than the picture: scale by height (cover), never stretched.
		expect(coverFrom({ left: 0, top: 0, width: 160, height: 180 }, 16 / 9, 1600, 900)).toContain('scale(0.2)');
	});
	it('clips to the rect with rounded corners', () => {
		expect(clipFrom({ left: 100, top: 50, width: 320, height: 180 }, 1600, 900, 12)).toBe('inset(50px 1180px 670px 100px round 12px)');
		expect(clipFrom({ left: -10, top: -10, width: 2000, height: 2000 }, 1600, 900, 0)).toBe('inset(0px 0px 0px 0px round 0px)');
	});
});
