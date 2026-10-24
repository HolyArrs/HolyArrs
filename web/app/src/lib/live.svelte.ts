// Live updates: one event stream per tab (/live). Each message is
// {"topic": "...", "data": ...}; pages subscribe to the topics they show.
// "resync" means the stream lost messages, so every listener reloads.
import { to } from './paths.ts';
import { api } from './api.ts';

export type LiveState = 'off' | 'connecting' | 'live' | 'reconnecting';
export type Listener = (data: unknown, topic: string) => void;

interface SourceLike {
	onopen: ((this: SourceLike, ev: Event) => unknown) | null;
	onerror: ((this: SourceLike, ev: Event) => unknown) | null;
	onmessage: ((this: SourceLike, ev: MessageEvent) => unknown) | null;
	close(): void;
}

export type SourceFactory = (url: string) => SourceLike;

import { backoff } from './loader.ts';

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
		// Something changed on the server: listeners must read fresh answers, not shared ones.
		api.forget();
		emit(msg.topic, msg.data);
	}

	let tries = 0;
	let retry: ReturnType<typeof setTimeout> | undefined;
	// A tab in the background lets go of its stream after a short while: the
	// browser opens only six connections to one address, so open streams in
	// background tabs would leave a new tab waiting forever. Back in front, the
	// stream opens again and every listener reloads (it may have missed messages).
	const PAUSE_AFTER = 10_000;
	let paused = false;
	let missed = false;
	let hideTimer: ReturnType<typeof setTimeout> | undefined;
	let watching = false;
	function visibility() {
		clearTimeout(hideTimer);
		if (document.hidden) {
			hideTimer = setTimeout(() => {
				if (!document.hidden || status.state === 'off') return;
				clearTimeout(retry);
				src?.close();
				src = null;
				paused = true;
			}, PAUSE_AFTER);
		} else if (paused) {
			paused = false;
			missed = true;
			start(true);
		}
	}

	function start(again = false) {
		if (src) return;
		if (!watching && typeof document !== 'undefined') {
			watching = true;
			document.addEventListener('visibilitychange', visibility);
			// A tab opened in the background never sees a change: start its countdown now.
			if (document.hidden) queueMicrotask(visibility);
		}
		if (!again) status.state = 'connecting';
		src = factory(to('/live'));
		src.onopen = () => {
			tries = 0;
			status.state = 'live';
			if (missed) {
				missed = false;
				api.forget();
				emit('resync', null);
			}
		};
		src.onerror = () => {
			// Lost the server (restart, update): retry after 1 s, 2 s, 4 s… up to 15 s.
			status.state = 'reconnecting';
			src?.close();
			src = null;
			clearTimeout(retry);
			retry = setTimeout(() => {
				if (status.state === 'reconnecting') start(true);
			}, backoff(tries++));
		};
		src.onmessage = (e: MessageEvent) => dispatch(String(e.data));
	}

	function stop() {
		clearTimeout(retry);
		clearTimeout(hideTimer);
		paused = false;
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

// Development: a hot update makes a new stream; the old one closes so open tabs do not pile up connections.
if (import.meta.hot) import.meta.hot.dispose(() => live.stop());
