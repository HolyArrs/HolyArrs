import { describe, expect, it, vi } from 'vitest';
import { FALLBACK_MS, nearView, whenIdle, whenLoaded } from './lazy-art.ts';

describe('nearView', () => {
	it('loads at once where there is no IntersectionObserver', () => {
		const near = vi.fn();
		nearView({} as Element, near);
		expect(near).toHaveBeenCalledOnce();
	});
	it('waits for the row to come near, then stops watching', () => {
		let fire: (es: { isIntersecting: boolean }[]) => void = () => {};
		const disconnect = vi.fn();
		const opts: IntersectionObserverInit[] = [];
		vi.stubGlobal('IntersectionObserver', class {
			constructor(cb: typeof fire, o: IntersectionObserverInit) { fire = cb; opts.push(o); }
			observe() {}
			disconnect = disconnect;
		});
		const near = vi.fn();
		nearView({} as Element, near);
		expect(opts[0].rootMargin).toBe('600px 0px');
		fire([{ isIntersecting: false }]);
		expect(near).not.toHaveBeenCalled();
		fire([{ isIntersecting: true }]);
		expect(near).toHaveBeenCalledOnce();
		expect(disconnect).toHaveBeenCalled();
		vi.unstubAllGlobals();
	});
});

describe('idle fallback', () => {
	it('loads a row whose observer never fires, once', () => {
		vi.useFakeTimers();
		vi.stubGlobal('IntersectionObserver', class {
			observe() {}
			disconnect() {}
		});
		const near = vi.fn();
		nearView({} as Element, near);
		vi.advanceTimersByTime(FALLBACK_MS + 50);
		expect(near).toHaveBeenCalledOnce();
		vi.unstubAllGlobals();
		vi.useRealTimers();
	});
	it('can be cancelled', () => {
		vi.useFakeTimers();
		const fn = vi.fn();
		whenIdle(fn, 100)();
		vi.advanceTimersByTime(500);
		expect(fn).not.toHaveBeenCalled();
		vi.useRealTimers();
	});
});

describe('whenLoaded', () => {
	it('is done at once for a picture already loaded', () => {
		const done = vi.fn();
		whenLoaded({ complete: true, naturalWidth: 342 } as HTMLImageElement, done);
		expect(done).toHaveBeenCalledOnce();
	});
	it('waits for load otherwise and cleans up', () => {
		const done = vi.fn();
		let on: () => void = () => {};
		const remove = vi.fn();
		const img = { complete: false, naturalWidth: 0, addEventListener: (_: string, f: () => void) => (on = f), removeEventListener: remove } as unknown as HTMLImageElement;
		const a = whenLoaded(img, done);
		expect(done).not.toHaveBeenCalled();
		on();
		expect(done).toHaveBeenCalledOnce();
		a.destroy?.();
		expect(remove).toHaveBeenCalled();
	});
});
