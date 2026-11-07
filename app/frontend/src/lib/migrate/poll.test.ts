import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createFollower } from './poll.ts';

type Answer = { id: number; done: boolean };

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

const flush = () => vi.advanceTimersByTimeAsync(0);

describe('createFollower', () => {
	it('asks every second until done, then ends once', async () => {
		let n = 0;
		const get = vi.fn(async (id: number) => ({ id, done: ++n >= 3 }));
		const end = vi.fn();
		const tick = vi.fn();
		const f = createFollower<Answer>({ get, done: (a) => a.done, onTick: tick, onEnd: end });
		f.follow(7);
		await flush();
		expect(get).toHaveBeenCalledTimes(1);
		await vi.advanceTimersByTimeAsync(1000);
		expect(get).toHaveBeenCalledTimes(2);
		await vi.advanceTimersByTimeAsync(1000);
		expect(get).toHaveBeenCalledTimes(3);
		expect(end).toHaveBeenCalledTimes(1);
		expect(tick).toHaveBeenCalledTimes(3);
		expect(f.following()).toBeNull();
		await vi.advanceTimersByTimeAsync(5000);
		expect(get).toHaveBeenCalledTimes(3);
	});

	it('runs only one loop: a new follow stops the old one', async () => {
		const get = vi.fn(async (id: number) => ({ id, done: false }));
		const f = createFollower<Answer>({ get, done: (a) => a.done });
		f.follow(1);
		f.follow(2);
		f.follow(2);
		await flush();
		await vi.advanceTimersByTimeAsync(3000);
		const ids = get.mock.calls.map((c) => c[0]);
		expect(ids.filter((i) => i === 1).length).toBeLessThanOrEqual(1);
		expect(ids.slice(-3)).toEqual([2, 2, 2]);
		expect(get.mock.calls.length).toBeLessThanOrEqual(6);
		f.destroy();
	});

	it('backs off 2s, 4s, then 10s after errors, and says when it is back', async () => {
		let calls = 0;
		const get = vi.fn(async (id: number) => {
			calls++;
			if (calls <= 4) throw new Error('down');
			return { id, done: false };
		});
		const lost = vi.fn();
		const back = vi.fn();
		const f = createFollower<Answer>({ get, done: (a) => a.done, onLost: lost, onBack: back });
		f.follow(3);
		await flush();
		expect(lost).toHaveBeenLastCalledWith(expect.any(Error), 1);
		await vi.advanceTimersByTimeAsync(1999);
		expect(get).toHaveBeenCalledTimes(1);
		await vi.advanceTimersByTimeAsync(1);
		expect(get).toHaveBeenCalledTimes(2);
		await vi.advanceTimersByTimeAsync(4000);
		expect(get).toHaveBeenCalledTimes(3);
		await vi.advanceTimersByTimeAsync(9999);
		expect(get).toHaveBeenCalledTimes(3);
		await vi.advanceTimersByTimeAsync(1);
		expect(get).toHaveBeenCalledTimes(4);
		await vi.advanceTimersByTimeAsync(10000);
		expect(get).toHaveBeenCalledTimes(5);
		expect(back).toHaveBeenCalledTimes(1);
		expect(lost).toHaveBeenCalledTimes(4);
		f.destroy();
	});

	it('hears nothing and asks nothing after destroy, even mid-ask', async () => {
		let release: (a: Answer) => void = () => {};
		const get = vi.fn(
			(id: number, signal: AbortSignal) =>
				new Promise<Answer>((resolve, reject) => {
					release = resolve;
					signal.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')));
					void id;
				})
		);
		const tick = vi.fn();
		const lost = vi.fn();
		const f = createFollower<Answer>({ get, done: (a) => a.done, onTick: tick, onLost: lost });
		f.follow(9);
		await flush();
		expect(get).toHaveBeenCalledTimes(1);
		const signal = get.mock.calls[0][1];
		f.destroy();
		expect(signal.aborted).toBe(true);
		release({ id: 9, done: false });
		await vi.advanceTimersByTimeAsync(20000);
		expect(get).toHaveBeenCalledTimes(1);
		expect(tick).not.toHaveBeenCalled();
		expect(lost).not.toHaveBeenCalled();
		f.follow(9);
		await flush();
		expect(get).toHaveBeenCalledTimes(1);
	});

	it('an answer that lands after a stop is dropped', async () => {
		let release: (a: Answer) => void = () => {};
		const get = vi.fn((id: number) => new Promise<Answer>((resolve) => ((release = resolve), void id)));
		const tick = vi.fn();
		const f = createFollower<Answer>({ get, done: (a) => a.done, onTick: tick });
		f.follow(4);
		await flush();
		f.stop();
		release({ id: 4, done: false });
		await vi.advanceTimersByTimeAsync(3000);
		expect(tick).not.toHaveBeenCalled();
		expect(get).toHaveBeenCalledTimes(1);
	});
});
