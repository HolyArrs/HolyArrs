import { coreIfSlow } from './music-pages.ts';
import { reaskWhileIncomplete, sameLists } from './reask.ts';
import { describe, expect, it, vi } from 'vitest';

describe('coreIfSlow', () => {
	it('skips the core when the full answer came in time', async () => {
		const core = vi.fn(() => Promise.resolve('core'));
		const r = await coreIfSlow(Promise.resolve('full'), core, 250, () => Promise.resolve());
		expect(r).toBeNull();
		expect(core).not.toHaveBeenCalled();
	});
	it('asks the core when the full answer is slow', async () => {
		const core = vi.fn(() => Promise.resolve('core'));
		const r = await coreIfSlow(new Promise(() => {}), core, 250, () => Promise.resolve());
		expect(r).toBe('core');
	});
	it('a failed full answer does not ask the core', async () => {
		const core = vi.fn(() => Promise.resolve('core'));
		const full = Promise.reject(new Error('x'));
		full.catch(() => {});
		expect(await coreIfSlow(full, core, 250, () => Promise.resolve())).toBeNull();
	});
});

describe('reaskWhileIncomplete with sameLists', () => {
	type A = { incomplete?: boolean; tracks: number[] };
	const same = sameLists<A>(['tracks']);
	it('stops once an answer brings nothing new, and shows it as final', async () => {
		const ask = vi.fn(async (): Promise<A> => ({ incomplete: true, tracks: [1] }));
		const took: boolean[] = [];
		const n = await reaskWhileIncomplete<A>({ incomplete: true, tracks: [1] }, ask, (_v, p) => took.push(p), new AbortController().signal, [1, 1, 1, 1], async () => {}, same);
		expect(n).toBe(1);
		expect(ask).toHaveBeenCalledTimes(1);
		expect(took).toEqual([true, false]);
	});
	it('keeps asking while answers grow', async () => {
		let k = 1;
		const ask = vi.fn(async (): Promise<A> => ({ incomplete: k < 3, tracks: Array.from({ length: ++k }, (_, i) => i) }));
		const n = await reaskWhileIncomplete<A>({ incomplete: true, tracks: [0] }, ask, () => {}, new AbortController().signal, [1, 1, 1, 1], async () => {}, same);
		expect(n).toBe(3);
	});
});
