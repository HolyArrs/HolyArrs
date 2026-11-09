import { describe, expect, it } from 'vitest';
import { prefetchQueue, type PrefetchTimers } from './trailer-prefetch.ts';

function fakeTimers() {
	let id = 0;
	const due = new Map<number, { at: number; f: () => void }>();
	let now = 0;
	const timers: PrefetchTimers = {
		set: (f, ms) => {
			due.set(++id, { at: now + ms, f });
			return id as unknown as ReturnType<typeof setTimeout>;
		},
		clear: (t) => void due.delete(t as unknown as number)
	};
	const advance = (ms: number) => {
		now += ms;
		for (const [k, d] of [...due].sort((a, b) => a[1].at - b[1].at)) {
			if (d.at <= now) {
				due.delete(k);
				d.f();
			}
		}
	};
	return { timers, advance };
}
const tick = () => new Promise((r) => setTimeout(r, 0));

describe('prefetchQueue', () => {
	it('gathers ids into one batch and ignores repeats', async () => {
		const sent: string[][] = [];
		const { timers, advance } = fakeTimers();
		const q = prefetchQueue({ send: async (ids) => (sent.push(ids), []), timers, gatherMs: 100 });
		q.add('movie:1');
		q.add('show:2');
		q.add('movie:1');
		expect(sent).toEqual([]);
		advance(100);
		expect(sent).toEqual([['movie:1', 'show:2']]);
		q.add('movie:1');
		advance(100);
		expect(sent.length).toBe(1);
	});
	it('sends a full batch at once and splits at max', () => {
		const sent: string[][] = [];
		const { timers } = fakeTimers();
		const q = prefetchQueue({ send: async (ids) => (sent.push(ids), []), timers, max: 2 });
		q.add('movie:1');
		q.add('movie:2');
		q.add('movie:3');
		expect(sent).toEqual([['movie:1', 'movie:2']]);
		expect(q.waiting).toBe(1);
	});
	it('asks a pending id once more, then stops', async () => {
		const sent: string[][] = [];
		const { timers, advance } = fakeTimers();
		const q = prefetchQueue({ send: async (ids) => (sent.push(ids), ids), timers, gatherMs: 10, retryMs: 1000 });
		q.add('movie:9');
		advance(10);
		await tick();
		advance(1000);
		advance(10);
		await tick();
		advance(1000);
		advance(10);
		expect(sent).toEqual([['movie:9'], ['movie:9']]);
	});
	it('survives a failed batch', async () => {
		const { timers, advance } = fakeTimers();
		const q = prefetchQueue({ send: () => Promise.reject(new Error('x')), timers, gatherMs: 1 });
		q.add('movie:1');
		advance(1);
		await tick();
		expect(q.waiting).toBe(0);
	});
});
