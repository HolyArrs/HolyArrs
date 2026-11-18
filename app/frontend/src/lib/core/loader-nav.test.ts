import { afterEach, describe, expect, it, vi } from 'vitest';

// The loader node the page change shows; hidden when the page is idle.
function fakeLoader() {
	const cls = new Set<string>();
	const node = { hidden: true, classList: { add: (c: string) => cls.add(c), remove: (...c: string[]) => c.forEach((x) => cls.delete(x)), contains: (c: string) => cls.has(c) } };
	vi.stubGlobal('document', { getElementById: (id: string) => (id === 'hl' ? node : null) });
	return node;
}

afterEach(() => {
	vi.useRealTimers();
	vi.unstubAllGlobals();
	vi.resetModules();
});

describe('a page change that a page cancels', () => {
	it('takes the veil away again, so the page stays usable', async () => {
		const node = fakeLoader();
		vi.useFakeTimers();
		const { navWatch, NAV_DELAY_MS } = await import('./loader.ts');
		let cancel: (e: Error) => void = () => {};
		const complete = new Promise<void>((_, no) => (cancel = no));
		navWatch(complete);
		// A "Leave anyway?" box holds the page longer than the delay.
		vi.advanceTimersByTime(NAV_DELAY_MS + 50);
		expect(node.hidden).toBe(false);
		cancel(new Error('navigation cancelled'));
		await vi.advanceTimersByTimeAsync(400);
		expect(node.hidden).toBe(true);
	});
	it('never shows the veil when cancelled at once', async () => {
		const node = fakeLoader();
		vi.useFakeTimers();
		const { navWatch, NAV_DELAY_MS } = await import('./loader.ts');
		navWatch(Promise.reject(new Error('navigation cancelled')));
		await vi.advanceTimersByTimeAsync(NAV_DELAY_MS + 400);
		expect(node.hidden).toBe(true);
	});
});

describe('a page change replaced by a newer one', () => {
	it('keeps the veil for the newer page change, which is still on its way', async () => {
		const node = fakeLoader();
		vi.useFakeTimers();
		const { navWatch, NAV_DELAY_MS } = await import('./loader.ts');
		let dropFirst: (e: Error) => void = () => {};
		// A second link is pressed while the first page is still loading: the first ends, another is under way.
		navWatch(new Promise<void>((_, no) => (dropFirst = no)), () => true);
		vi.advanceTimersByTime(NAV_DELAY_MS + 50);
		expect(node.hidden).toBe(false);
		dropFirst(new Error('navigation aborted'));
		await vi.advanceTimersByTimeAsync(400);
		expect(node.hidden).toBe(false);
	});
	it('still takes the veil away when no page change is under way any more', async () => {
		const node = fakeLoader();
		vi.useFakeTimers();
		const { navWatch, NAV_DELAY_MS } = await import('./loader.ts');
		let drop: (e: Error) => void = () => {};
		navWatch(new Promise<void>((_, no) => (drop = no)), () => false);
		vi.advanceTimersByTime(NAV_DELAY_MS + 50);
		expect(node.hidden).toBe(false);
		drop(new Error('navigation cancelled'));
		await vi.advanceTimersByTimeAsync(400);
		expect(node.hidden).toBe(true);
	});
	it('is told by the frame whether another page change is under way', async () => {
		const { readFileSync } = await import('node:fs');
		const frame = readFileSync(new URL('../../routes/+layout.svelte', import.meta.url), 'utf8');
		expect(frame).toContain('navWatch(nav.complete, () => navigating.to !== null)');
	});
});
