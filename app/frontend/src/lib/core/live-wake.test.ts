import { afterEach, describe, expect, it, vi } from 'vitest';
import { createLive, type SourceFactory } from './live.svelte.ts';

// A page that slept or froze comes back to a dead stream; it must reopen at once, quietly.
function fakePage() {
	const doc = new EventTarget() as EventTarget & { hidden: boolean };
	doc.hidden = false;
	const win = new EventTarget();
	vi.stubGlobal('document', doc);
	vi.stubGlobal('window', win);
	return { doc, win };
}

function fakeSources() {
	const made: { onopen: null | ((e: Event) => void); onerror: null | ((e: Event) => void); onmessage: null | ((e: MessageEvent) => void); close: () => void }[] = [];
	const factory = (() => {
		const s = { onopen: null, onerror: null, onmessage: null, close: vi.fn() };
		made.push(s);
		return s;
	}) as unknown as SourceFactory;
	return { made, factory };
}

afterEach(() => {
	vi.unstubAllGlobals();
	vi.useRealTimers();
});

describe('live after the page wakes', () => {
	it('reopens at once and stays quiet when no beat came for a while', () => {
		vi.useFakeTimers();
		const { win } = fakePage();
		const { made, factory } = fakeSources();
		const live = createLive(factory);
		live.start();
		made[0].onopen?.(new Event('open'));
		// Timers stood still while the page was frozen, but the clock moved on.
		vi.setSystemTime(Date.now() + 5 * 60_000);
		win.dispatchEvent(new Event('pageshow'));
		expect(made.length).toBe(2);
		expect(made[0].close).toHaveBeenCalled();
		expect(live.status.state).toBe('live');
		made[1].onopen?.(new Event('open'));
		expect(live.status.state).toBe('live');
		live.stop();
	});

	it('leaves a healthy stream alone on focus', () => {
		vi.useFakeTimers();
		const { win } = fakePage();
		const { made, factory } = fakeSources();
		const live = createLive(factory);
		live.start();
		made[0].onopen?.(new Event('open'));
		vi.advanceTimersByTime(10_000);
		win.dispatchEvent(new Event('focus'));
		expect(made.length).toBe(1);
		live.stop();
	});

	it('retries now when back online instead of waiting out the backoff', () => {
		vi.useFakeTimers();
		const { win } = fakePage();
		const { made, factory } = fakeSources();
		const live = createLive(factory);
		live.start();
		made[0].onopen?.(new Event('open'));
		made[0].onerror?.(new Event('error'));
		vi.advanceTimersByTime(0);
		made[1].onerror?.(new Event('error'));
		expect(live.status.state).toBe('reconnecting');
		win.dispatchEvent(new Event('online'));
		expect(made.length).toBe(3);
		live.stop();
	});

	it('treats a stall timer that fires late as a wake, not a hang', () => {
		vi.useFakeTimers();
		fakePage();
		const { made, factory } = fakeSources();
		const live = createLive(factory);
		live.start();
		made[0].onopen?.(new Event('open'));
		// Laptop lid closed: the clock jumps, then the old timer finally fires.
		vi.setSystemTime(Date.now() + 10 * 60_000);
		vi.advanceTimersByTime(35_000);
		expect(made.length).toBe(2);
		expect(live.status.state).toBe('live');
		live.stop();
	});

	it('still shows reconnecting for a real hang', () => {
		vi.useFakeTimers();
		fakePage();
		const { made, factory } = fakeSources();
		const live = createLive(factory);
		live.start();
		made[0].onopen?.(new Event('open'));
		vi.advanceTimersByTime(35_000);
		expect(live.status.state).toBe('reconnecting');
		live.stop();
	});
});
