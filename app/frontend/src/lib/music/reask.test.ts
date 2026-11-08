import { GAP_REASK_MS, gapWaits, keepShown, reaskWhileIncomplete, REASK_MS, sleep } from './reask.ts';
import { describe, expect, it, vi } from 'vitest';

type A = { tracks: number[]; incomplete?: boolean };
const now = () => Promise.resolve();

describe('reaskWhileIncomplete', () => {
	it('asks again until the answer is complete', async () => {
		const answers: A[] = [{ tracks: [1, 2], incomplete: true }, { tracks: [1, 2, 3] }];
		const ask = vi.fn(async () => answers.shift() as A);
		const seen: [number, boolean][] = [];
		const waits: number[] = [];
		const n = await reaskWhileIncomplete<A>({ tracks: [1], incomplete: true }, ask, (v, p) => seen.push([v.tracks.length, p]), new AbortController().signal, REASK_MS, async (ms) => void waits.push(ms));
		expect(n).toBe(2);
		expect(seen).toEqual([[1, true], [2, true], [3, false]]);
		expect(waits).toEqual([2500, 3500]);
	});

	it('does nothing more for a complete first answer', async () => {
		const ask = vi.fn();
		const take = vi.fn();
		expect(await reaskWhileIncomplete<A>({ tracks: [1] }, ask, take, new AbortController().signal, REASK_MS, now)).toBe(0);
		expect(ask).not.toHaveBeenCalled();
		expect(take).toHaveBeenCalledWith({ tracks: [1] }, false);
	});

	it('stops after the last try and says nothing more is coming', async () => {
		const ask = vi.fn(async (): Promise<A> => ({ tracks: [], incomplete: true }));
		const pending: boolean[] = [];
		const n = await reaskWhileIncomplete<A>({ tracks: [], incomplete: true }, ask, (_, p) => pending.push(p), new AbortController().signal, [1, 2, 3, 4], now);
		expect(n).toBe(4);
		expect(ask).toHaveBeenCalledTimes(4);
		expect(pending).toEqual([true, true, true, true, false]);
	});

	it('ends pending after a last re-ask that failed', async () => {
		const ask = vi.fn(async (): Promise<A> => {
			throw new Error('down');
		});
		const pending: boolean[] = [];
		await reaskWhileIncomplete<A>({ tracks: [1], incomplete: true }, ask, (_, p) => pending.push(p), new AbortController().signal, [1, 2], now);
		expect(pending).toEqual([true, false]);
	});

	it('stops when the page is left', async () => {
		vi.useFakeTimers();
		try {
			const ask = vi.fn(async (): Promise<A> => ({ tracks: [], incomplete: true }));
			const stop = new AbortController();
			const done = reaskWhileIncomplete<A>({ tracks: [], incomplete: true }, ask, () => {}, stop.signal);
			await vi.advanceTimersByTimeAsync(2600);
			expect(ask).toHaveBeenCalledTimes(1);
			stop.abort();
			await vi.advanceTimersByTimeAsync(30_000);
			expect(ask).toHaveBeenCalledTimes(1);
			expect(await done).toBe(1);
		} finally {
			vi.useRealTimers();
		}
	});

	it('sleep rejects at once on abort', async () => {
		const c = new AbortController();
		const p = sleep(10_000, c.signal);
		c.abort();
		await expect(p).rejects.toBeDefined();
	});
});

describe('keepShown', () => {
	it('keeps longer lists on screen while the newer answer is incomplete', () => {
		expect(keepShown<A>({ tracks: [1, 2] }, { tracks: [1], incomplete: true }, ['tracks']).tracks).toEqual([1, 2]);
		expect(keepShown<A>({ tracks: [1] }, { tracks: [1, 2, 3], incomplete: true }, ['tracks']).tracks).toEqual([1, 2, 3]);
		expect(keepShown<A>({ tracks: [1, 2] }, { tracks: [1] }, ['tracks']).tracks).toEqual([1]);
		expect(keepShown<A>(null, { tracks: [1] }, ['tracks']).tracks).toEqual([1]);
	});
});

describe('song list state', () => {
	it('shows skeleton rows while pending and empty, rows once songs arrive, a neutral line after giving up', async () => {
		const { trackListView, sameTracks, TRACK_REASK_MS } = await import('./reask.ts');
		expect(trackListView(0, true)).toBe('skeleton');
		expect(trackListView(0, false)).toBe('gave-up');
		expect(trackListView(3, true)).toBe('rows');
		expect(trackListView(3, false)).toBe('rows');
		expect(TRACK_REASK_MS.reduce((a, b) => a + b, 0)).toBeGreaterThanOrEqual(55000);
		expect(TRACK_REASK_MS.reduce((a, b) => a + b, 0)).toBeLessThanOrEqual(65000);
		// Quick first re-asks: a list ready at 9 s is asked again by about 9 s (each ask held up to 0.8 s).
		expect(TRACK_REASK_MS.slice(0, 3)).toEqual([1000, 1500, 2000]);
		let at = 0.8;
		let i = 0;
		while (at < 9) at += TRACK_REASK_MS[i++] / 1000 + 0.8;
		expect(at).toBeLessThanOrEqual(10.5);
		expect(sameTracks({ tracks: [] }, { tracks: [] })).toBe(false);
		expect(sameTracks({ tracks: [1] }, { tracks: [2] })).toBe(true);
	});

	it('keeps asking an empty list until the pauses run out, then stops pending', async () => {
		const { TRACK_REASK_MS, sameTracks } = await import('./reask.ts');
		const ask = vi.fn(async () => ({ tracks: [] as number[], incomplete: true }));
		const seen: boolean[] = [];
		const n = await reaskWhileIncomplete({ tracks: [] as number[], incomplete: true }, ask, (_v, p) => seen.push(p), new AbortController().signal, TRACK_REASK_MS, now, sameTracks);
		expect(n).toBe(TRACK_REASK_MS.length);
		expect(seen[0]).toBe(true);
		expect(seen[seen.length - 1]).toBe(false);
	});
});

describe('gapWaits', () => {
	it('keeps asking for minutes while the server says it is still getting the item', () => {
		expect(gapWaits({ gap: { state: 'queued' } })).toBe(GAP_REASK_MS);
		expect(GAP_REASK_MS.reduce((a, b) => a + b, 0)).toBeGreaterThanOrEqual(120000);
		expect(gapWaits({})).toBe(REASK_MS);
	});
});
