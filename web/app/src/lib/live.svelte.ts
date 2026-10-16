// Live updates: one event stream per tab (/live). Each message is
// {"topic": "...", "data": ...}; pages subscribe to the topics they show.
// "resync" means the stream lost messages, so every listener reloads.
import { to } from './paths.ts';

export type LiveState = 'off' | 'connecting' | 'live' | 'reconnecting';
export type Listener = (data: unknown, topic: string) => void;

interface SourceLike {
	onopen: ((this: SourceLike, ev: Event) => unknown) | null;
	onerror: ((this: SourceLike, ev: Event) => unknown) | null;
	onmessage: ((this: SourceLike, ev: MessageEvent) => unknown) | null;
	close(): void;
}

export type SourceFactory = (url: string) => SourceLike;

export function createLive(factory: SourceFactory = (u) => new EventSource(u) as unknown as SourceLike) {
	const status = $state<{ state: LiveState; last: number }>({ state: 'off', last: 0 });
	const listeners = new Map<string, Set<Listener>>();
	let src: SourceLike | null = null;

	function emit(topic: string, data: unknown) {
		const run = (set: Set<Listener> | undefined) => set?.forEach((fn) => fn(data, topic));
		if (topic === 'resync') {
			for (const [t, set] of listeners) if (t !== '*') run(set);
		} else {
			run(listeners.get(topic));
		}
		run(listeners.get('*'));
	}

	/** dispatch handles one raw message; exported for tests. */
	function dispatch(raw: string) {
		let msg: { topic?: unknown; data?: unknown };
		try {
			msg = JSON.parse(raw);
		} catch {
			return;
		}
		if (typeof msg.topic !== 'string') return;
		status.last = Date.now();
		emit(msg.topic, msg.data);
	}

	function start() {
		if (src) return;
		status.state = 'connecting';
		src = factory(to('/live'));
		src.onopen = () => {
			status.state = 'live';
		};
		src.onerror = () => {
			// EventSource retries by itself and sends Last-Event-ID on resume.
			status.state = 'reconnecting';
		};
		src.onmessage = (e: MessageEvent) => dispatch(String(e.data));
	}

	function stop() {
		src?.close();
		src = null;
		status.state = 'off';
	}

	/** on listens to a topic ("*" for all) and returns the way to stop. */
	function on(topic: string, fn: Listener): () => void {
		let set = listeners.get(topic);
		if (!set) listeners.set(topic, (set = new Set()));
		set.add(fn);
		return () => set.delete(fn);
	}

	return { status, start, stop, on, dispatch };
}

export const live = createLive();
