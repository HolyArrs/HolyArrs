import { describe, expect, it, vi } from 'vitest';
import { createLive, type SourceFactory } from './live.svelte.ts';

function fakeSources() {
	const made: {
		url: string;
		onopen: null | ((e: Event) => void);
		onerror: null | ((e: Event) => void);
		onmessage: null | ((e: MessageEvent) => void);
		close: () => void;
	}[] = [];
	const factory = ((url: string) => {
		const s = { url, onopen: null, onerror: null, onmessage: null, close: vi.fn() };
		made.push(s);
		return s;
	}) as unknown as SourceFactory;
	return { made, factory };
}

const msg = (topic: string, id: string) => new MessageEvent('message', { data: JSON.stringify({ topic, data: {} }), lastEventId: id });

describe('live resume', () => {
	it('reopens with ?last= after an error and lets the server decide', () => {
		vi.useFakeTimers();
		const { made, factory } = fakeSources();
		const live = createLive(factory);
		const title = vi.fn();
		live.on('title', title);
		live.start();
		made[0].onopen?.(new Event('open'));
		made[0].onmessage?.(msg('title', 'b00t-7'));
		made[0].onerror?.(new Event('error'));
		vi.advanceTimersByTime(20_000);
		expect(made[1].url).toBe('/live?last=b00t-7');
		made[1].onopen?.(new Event('open'));
		expect(title).toHaveBeenCalledTimes(1);
		live.stop();
		vi.useRealTimers();
	});

	it('reloads every listener when the boot id changes (server restart)', () => {
		const { made, factory } = fakeSources();
		const live = createLive(factory);
		const job = vi.fn();
		const ask = vi.fn();
		live.on('job', job);
		live.on('ask', ask);
		live.start();
		made[0].onmessage?.(msg('job', 'aaaa-1'));
		made[0].onmessage?.(msg('job', 'aaaa-2'));
		expect(ask).not.toHaveBeenCalled();
		made[0].onmessage?.(msg('job', 'bbbb-1'));
		expect(ask).toHaveBeenCalledWith(null, 'resync');
		expect(ask).toHaveBeenCalledTimes(1);
		live.stop();
	});

	it('reloads after a reconnect when there was no id to resume from', () => {
		vi.useFakeTimers();
		const { made, factory } = fakeSources();
		const live = createLive(factory);
		const title = vi.fn();
		live.on('title', title);
		live.start();
		made[0].onopen?.(new Event('open'));
		made[0].onerror?.(new Event('error'));
		vi.advanceTimersByTime(20_000);
		expect(made[1].url).toBe('/live');
		made[1].onopen?.(new Event('open'));
		expect(title).toHaveBeenCalledWith(null, 'resync');
		live.stop();
		vi.useRealTimers();
	});
});
