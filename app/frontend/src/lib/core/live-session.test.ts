import { afterEach, describe, expect, it, vi } from 'vitest';
import { api, ApiError } from './api.ts';
import { session } from './session.svelte.ts';
import { createLive } from './live.svelte.ts';

type Src = { onopen: (() => void) | null; onerror: (() => void) | null; onmessage: null; close(): void };

function setup() {
	const made: Src[] = [];
	const l = createLive(() => {
		const s: Src = { onopen: null, onerror: null, onmessage: null, close() {} };
		made.push(s);
		return s as never;
	});
	return { l, made };
}

describe('live stream and an ended session', () => {
	afterEach(() => {
		vi.restoreAllMocks();
		vi.useRealTimers();
	});

	it('signs out when the stream dies because the session ended', async () => {
		vi.useFakeTimers();
		vi.spyOn(api, 'get').mockRejectedValue(new ApiError('gone', 401));
		const lost = vi.spyOn(session, 'lost').mockImplementation(() => {});
		const { l, made } = setup();
		l.start();
		made[0].onerror?.();
		await vi.runAllTicks();
		await Promise.resolve();
		expect(lost).toHaveBeenCalledTimes(1);
		expect(l.status.state).toBe('off');
	});

	it('checks again on a later error, so a session lost during an outage is caught', async () => {
		vi.useFakeTimers();
		const get = vi.spyOn(api, 'get').mockRejectedValueOnce(new ApiError('down', 0)).mockRejectedValue(new ApiError('gone', 401));
		const lost = vi.spyOn(session, 'lost').mockImplementation(() => {});
		const { l, made } = setup();
		l.start();
		made[0].onerror?.();
		await Promise.resolve();
		await Promise.resolve();
		// The reopened stream goes quiet, so the stall check raises the next error.
		await vi.advanceTimersByTimeAsync(120_000);
		expect(get.mock.calls.length).toBeGreaterThanOrEqual(2);
		expect(lost).toHaveBeenCalledTimes(1);
		expect(l.status.state).toBe('off');
	});

	it('runs only one check while one is still waiting', async () => {
		vi.useFakeTimers();
		const get = vi.spyOn(api, 'get').mockReturnValue(new Promise(() => {}));
		const { l, made } = setup();
		l.start();
		made[0].onerror?.();
		await vi.advanceTimersByTimeAsync(60_000);
		made[made.length - 1].onerror?.();
		expect(made.length).toBeGreaterThan(1);
		expect(get).toHaveBeenCalledTimes(1);
	});
});
