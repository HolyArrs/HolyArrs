import { afterEach, describe, expect, it, vi } from 'vitest';
import { createLive, type SourceFactory } from './live.svelte.ts';

// A stand-in document whose hidden flag the test flips.
function fakeDocument() {
	const doc = new EventTarget() as EventTarget & { hidden: boolean };
	doc.hidden = false;
	vi.stubGlobal('document', doc);
	return (hidden: boolean) => {
		doc.hidden = hidden;
		doc.dispatchEvent(new Event('visibilitychange'));
	};
}

function fakeSources() {
	const made: { onopen: null | ((e: Event) => void); onerror: null | ((e: Event) => void); onmessage: unknown; close: () => void }[] = [];
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

describe('live after a hidden tab comes back', () => {
	it('retries at once instead of waiting out the old backoff', () => {
		vi.useFakeTimers();
		const show = fakeDocument();
		const { made, factory } = fakeSources();
		const live = createLive(factory);
		live.start();
		made[0].onopen?.(new Event('open'));
		show(true);
		// The connection keeps failing while the tab is in the background.
		for (let i = 0; i < 3; i++) {
			made[made.length - 1].onerror?.(new Event('error'));
			vi.advanceTimersByTime(1_000 * 2 ** i);
		}
		vi.advanceTimersByTime(10_000);
		const before = made.length;
		show(false);
		expect(made.length).toBe(before + 1);
		// The first reopen fails once; the next try comes after the shortest wait, not 15 s.
		made[made.length - 1].onerror?.(new Event('error'));
		vi.advanceTimersByTime(1_000);
		expect(made.length).toBe(before + 2);
		live.stop();
	});
});
