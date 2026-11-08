import { nextBar, overHero, type BarState } from './scroll-bar.svelte.ts';
import { describe, expect, it } from 'vitest';

const walk = (ys: number[], s: BarState = { hidden: false, anchor: 0 }) => ys.reduce(nextBar, s);

describe('nextBar', () => {
	it('stays shown under 80px of downward scroll', () => expect(walk([20, 50, 79]).hidden).toBe(false));
	it('hides after 80px down', () => expect(walk([40, 80]).hidden).toBe(true));
	it('stays hidden on a small upward wobble', () => expect(walk([100, 300, 290, 300, 280]).hidden).toBe(true));
	it('shows after 24px up', () => expect(walk([100, 300, 276]).hidden).toBe(false));
	it('always shows at the top', () => expect(walk([100, 300, 0]).hidden).toBe(false));
	it('measures the hide run from the lowest point after scrolling up', () => {
		const s = walk([100, 300, 200]);
		expect(s.hidden).toBe(false);
		expect(walk([270], s).hidden).toBe(false);
		expect(walk([280], s).hidden).toBe(true);
	});
});


describe('overHero', () => {
	it('is see-through while the hero shows under the bar', () => expect(overHero(600, 64)).toBe(true));
	it('gets its glass back once the hero has scrolled by', () => expect(overHero(70, 64)).toBe(false));
	it('pages without a hero keep the glass', () => expect(overHero(null, 64)).toBe(false));
});
