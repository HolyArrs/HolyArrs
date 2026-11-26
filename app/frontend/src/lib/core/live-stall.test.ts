import { describe, expect, it, vi } from 'vitest';
import { createLive, type SourceFactory } from './live.svelte.ts';

function fakeSources() {
	const made: { url: string; onopen: null | ((e: Event) => void); onerror: null | ((e: Event) => void); onmessage: null | ((e: MessageEvent) => void); close: () => void }[] = [];
	const factory = ((url: string) => {
		const s = { url, onopen: null, onerror: null, onmessage: null, close: vi.fn() };
		made.push(s);
		return s;
	}) as unknown as SourceFactory;
	return { made, factory };
}

const beat = () => new MessageEvent('message', { data: '{}', lastEventId: '' });

describe('live stall', () => {
	it('shows reconnecting when a stream that stays open goes silent', () => {
		vi.useFakeTimers();
		const { made, factory } = fakeSources();
		const live = createLive(factory);
		live.start();
		made[0].onopen?.(new Event('open'));
		vi.advanceTimersByTime(30_000);
		made[0].onmessage?.(beat());
		vi.advanceTimersByTime(30_000);
		expect(live.status.state).toBe('live');
		vi.advanceTimersByTime(20_000);
		expect(live.status.state).toBe('reconnecting');
		expect(made[0].close).toHaveBeenCalled();
		vi.advanceTimersByTime(20_000);
		expect(made.length).toBe(2);
		made[1].onopen?.(new Event('open'));
		expect(live.status.state).toBe('live');
		live.stop();
		vi.useRealTimers();
	});
});
