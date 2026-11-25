import { beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('./api.ts', () => {
	class ApiError extends Error {
		status: number;
		constructor(message: string, status: number) {
			super(message);
			this.name = 'ApiError';
			this.status = status;
		}
	}
	return { api: { get: vi.fn(), forget: vi.fn() }, ApiError };
});
vi.mock('./session.svelte.ts', () => ({ session: { lost: vi.fn() } }));

import { createLive, type SourceFactory } from './live.svelte.ts';
import { api, ApiError } from './api.ts';
import { session } from './session.svelte.ts';

function fakeSources() {
	const made: { url: string; onopen: null | ((e: Event) => void); onerror: null | ((e: Event) => void); onmessage: null | ((e: MessageEvent) => void); close: () => void }[] = [];
	const factory = ((url: string) => {
		const s = { url, onopen: null, onerror: null, onmessage: null, close: vi.fn() };
		made.push(s);
		return s;
	}) as unknown as SourceFactory;
	return { made, factory };
}

describe('live session expiry', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		vi.useFakeTimers();
	});

	it('signs out instead of reconnecting when the session ended (401)', async () => {
		vi.mocked(api.get).mockRejectedValue(new ApiError('sign in first', 401));
		const { made, factory } = fakeSources();
		const live = createLive(factory);
		live.start();
		made[0].onopen?.(new Event('open'));
		expect(live.status.state).toBe('live');
		made[0].onerror?.(new Event('error'));
		// Let the session probe settle before any retry timer runs.
		await Promise.resolve();
		expect(session.lost).toHaveBeenCalledTimes(1);
		expect(live.status.state).toBe('off');
		vi.advanceTimersByTime(20_000);
		expect(made.length).toBe(1);
		live.stop();
		vi.useRealTimers();
	});

	it('reconnects when the session is still good', async () => {
		vi.mocked(api.get).mockResolvedValue(undefined);
		const { made, factory } = fakeSources();
		const live = createLive(factory);
		live.start();
		made[0].onopen?.(new Event('open'));
		made[0].onerror?.(new Event('error'));
		await Promise.resolve();
		expect(session.lost).not.toHaveBeenCalled();
		vi.advanceTimersByTime(20_000);
		expect(made.length).toBe(2);
		made[1].onopen?.(new Event('open'));
		expect(live.status.state).toBe('live');
		live.stop();
		vi.useRealTimers();
	});

	it('reconnects when the probe cannot reach the server', async () => {
		vi.mocked(api.get).mockRejectedValue(new ApiError('not answering', 0));
		const { made, factory } = fakeSources();
		const live = createLive(factory);
		live.start();
		made[0].onopen?.(new Event('open'));
		made[0].onerror?.(new Event('error'));
		await Promise.resolve();
		expect(session.lost).not.toHaveBeenCalled();
		vi.advanceTimersByTime(20_000);
		expect(made.length).toBe(2);
		live.stop();
		vi.useRealTimers();
	});
});
