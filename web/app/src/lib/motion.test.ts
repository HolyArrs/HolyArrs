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
