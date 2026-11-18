import { afterEach, describe, expect, it, vi } from 'vitest';

type Listener = (data: unknown, topic: string) => void;
const listeners = new Map<string, Set<Listener>>();

vi.mock('$app/navigation', () => ({ goto: vi.fn() }));
vi.mock('#lib/core/api.ts', () => ({ api: { get: vi.fn(() => new Promise(() => {})), post: vi.fn() } }));
vi.mock('#lib/core/live.svelte.ts', () => ({
	live: {
		on(topic: string, fn: Listener) {
			const set = listeners.get(topic) ?? new Set<Listener>();
			set.add(fn);
			listeners.set(topic, set);
			return () => set.delete(fn);
		}
	}
}));
vi.mock('#lib/downloads/index.ts', () => ({ decisions: { items: [] } }));
vi.mock('#lib/core/inbox.svelte.ts', () => ({ inbox: { messages: [] } }));
vi.mock('#lib/core/toasts.svelte.ts', () => ({ toasts: { push: vi.fn(), record: vi.fn() } }));

const { startBellFeeds } = await import('./bell-feeds.svelte.ts');
const { toasts } = await import('#lib/core/toasts.svelte.ts');

// The live stream hands every topic listener null on a resync (server restart, missed events).
const fire = (data: unknown) => listeners.get('download')?.forEach((fn) => fn(data, 'resync'));

describe('bell download feed', () => {
	let stop: (() => void) | undefined;
	afterEach(() => {
		stop?.();
		vi.clearAllMocks();
	});

	it('ignores the empty event a resync sends', () => {
		stop = startBellFeeds(true);
		expect(() => fire(null)).not.toThrow();
		expect(() => fire(undefined)).not.toThrow();
		expect(toasts.push).not.toHaveBeenCalled();
	});

	it('still pops up a failed download', () => {
		stop = startBellFeeds(true);
		fire({ State: 'failed', StateReason: 'no seeders' });
		expect(toasts.push).toHaveBeenCalledTimes(1);
	});
});
