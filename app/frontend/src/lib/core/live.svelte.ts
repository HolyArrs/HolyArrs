// Server event stream with reconnect, resync and one stream shared across tabs.

import { to } from './paths.ts';
import { api } from './api.ts';
import { backoff } from './loader.ts';

export type LiveState = 'off' | 'connecting' | 'live' | 'reconnecting';
export type Listener = (data: unknown, topic: string) => void;

interface SourceLike {
	onopen: ((this: SourceLike, ev: Event) => unknown) | null;
	onerror: ((this: SourceLike, ev: Event) => unknown) | null;
	onmessage: ((this: SourceLike, ev: MessageEvent) => unknown) | null;
	close(): void;
}

export type SourceFactory = (url: string) => SourceLike;

export interface LocksLike {
	request(name: string, opts: { signal?: AbortSignal }, fn: () => Promise<void>): Promise<unknown>;
}
export interface ChannelLike {
	onmessage: ((ev: MessageEvent) => unknown) | null;
	postMessage(msg: unknown): void;
	close(): void;
}
export type Shared = { locks: LocksLike; channel: () => ChannelLike; post?: (view: string) => Promise<unknown> };

type Relay =
	| { t: 'ev'; id: string; data: string }
	| { t: 'state'; state: LiveState; last: string }
	| { t: 'hello' }
	| { t: 'interest'; view: string };

// Only one tab holds this lock and owns the stream; the others get events relayed.
const LOCK = 'holyarrs-live';

// Builds a stream; the factory and shared parts can be swapped so tests can fake them.
export function createLive(factory: SourceFactory = (u) => new EventSource(u) as unknown as SourceLike, shared?: Shared) {
	const status = $state<{ state: LiveState; last: number; leader: boolean }>({ state: 'off', last: 0, leader: false });
	const listeners = new Map<string, Set<Listener>>();
	let src: SourceLike | null = null;
	let lastId = '';

	// A new boot id in the event id means the server restarted, so cached data is stale.
	function seen(id: string) {
		if (!id) return;
		const boot = id.slice(0, id.lastIndexOf('-'));
		const before = lastId.slice(0, lastId.lastIndexOf('-'));
		lastId = id;
		if (before && boot !== before) {
			api.forget();
			emit('resync', null);
		}
	}

	// A resync reaches every topic listener except the catch-all, which gets every event after.
	function emit(topic: string, data: unknown) {
		const run = (set: Set<Listener> | undefined) => set?.forEach((fn) => fn(data, topic));
		if (topic === 'resync') {
			for (const [t, set] of listeners) if (t !== '*') run(set);
		} else {
			run(listeners.get(topic));
		}
		run(listeners.get('*'));
	}

	function dispatch(raw: string) {
		let msg: { topic?: unknown; data?: unknown };
		try {
			msg = JSON.parse(raw);
		} catch {
			return;
		}
		if (typeof msg.topic !== 'string') return;
		status.last = Date.now();
		api.forget();
		emit(msg.topic, msg.data);
	}

	let tries = 0;
	let retry: ReturnType<typeof setTimeout> | undefined;
	const PAUSE_AFTER = 10_000;
	let paused = false;
	let missed = false;
	let hideTimer: ReturnType<typeof setTimeout> | undefined;
	let watching = false;
	// A tab hidden for 10 seconds drops its stream and reopens when shown again.
	function visibility() {
		clearTimeout(hideTimer);
		if (document.hidden) {
			hideTimer = setTimeout(() => {
				if (!document.hidden || status.state === 'off') return;
				paused = true;
				if (shared) return withdraw();
				clearTimeout(retry);
				src?.close();
				src = null;
			}, PAUSE_AFTER);
		} else if (paused) {
			paused = false;
			missed = !lastId;
			if (!shared) return open(true);
			if (!joined) return;
			checkLast = true;
			send({ t: 'hello' });
			lead();
		}
	}

	function watch() {
		if (!watching && typeof document !== 'undefined') {
			watching = true;
			document.addEventListener('visibilitychange', visibility);
			if (document.hidden) queueMicrotask(visibility);
		}
	}

	function start() {
		watch();
		if (!shared) return open();
		if (joined) return;
		joined = true;
		status.state = 'connecting';
		if (!chan) {
			chan = shared.channel();
			chan.onmessage = (e) => onRelay(e.data as Relay);
		}
		send({ t: 'hello' });
		if (!hidden()) lead();
	}

	function open(again = false) {
		if (src) return;
		if (!again) setState('connecting');
		src = factory(to('/live') + (lastId ? '?last=' + encodeURIComponent(lastId) : ''));
		src.onopen = () => {
			tries = 0;
			setState('live');
			if (missed) {
				missed = false;
				api.forget();
				emit('resync', null);
			}
		};
		src.onerror = () => {
			if (!lastId && status.state === 'live') missed = true;
			const quiet = status.state === 'live' && tries === 0;
			if (!quiet) setState('reconnecting');
			src?.close();
			src = null;
			clearTimeout(retry);
			retry = setTimeout(() => {
				if ((quiet ? status.state === 'live' : status.state === 'reconnecting') && (!shared || status.leader)) open(true);
			}, quiet ? 0 : backoff(tries));
			tries++;
		};
		src.onmessage = (e: MessageEvent) => {
			const id = e.lastEventId ?? '';
			if (shared) send({ t: 'ev', id, data: String(e.data) });
			seen(id);
			dispatch(String(e.data));
		};
	}

	let chan: ChannelLike | null = null;
	let joined = false;
	let release: (() => void) | null = null;
	let abort: AbortController | null = null;
	let checkLast = false;
	const hidden = () => typeof document !== 'undefined' && document.hidden;

	function send(m: Relay) {
		chan?.postMessage(m);
	}
	function setState(st: LiveState) {
		status.state = st;
		if (status.leader) send({ t: 'state', state: st, last: lastId });
	}
	function onRelay(m: Relay) {
		if (!m || typeof m !== 'object' || !joined) return;
		if (m.t === 'ev' && !status.leader) {
			seen(m.id);
			dispatch(m.data);
		} else if (m.t === 'state' && !status.leader) {
			status.state = m.state;
			if (checkLast && m.state === 'live') {
				checkLast = false;
				if (m.last !== lastId) {
					if (m.last) lastId = m.last;
					api.forget();
					emit('resync', null);
				}
			}
		} else if (m.t === 'hello' && status.leader) {
			send({ t: 'state', state: status.state, last: lastId });
		} else if (m.t === 'interest' && status.leader) {
			postInterest(m.view);
		}
	}
	// Wins the lock to own the stream; losing tabs wait and take over when the leader closes.
	function lead() {
		if (!shared || release || abort) return;
		const ctl = (abort = new AbortController());
		shared.locks
			.request(LOCK, { signal: ctl.signal }, () => {
				abort = null;
				status.leader = true;
				checkLast = false;
				open(status.state === 'live');
				return new Promise<void>((done) => {
					release = () => {
						release = null;
						status.leader = false;
						clearTimeout(retry);
						src?.close();
						src = null;
						done();
					};
				});
			})
			.catch(() => {})
			.finally(() => {
				if (abort === ctl) abort = null;
			});
	}
	function withdraw() {
		abort?.abort();
		abort = null;
		release?.();
	}
	// Tells the server which view is open so it can send matching events.
	const sentAt = new Map<string, number>();
	// At most one interest post per view every 10 seconds.
	function postInterest(view: string) {
		const now = Date.now();
		if (now - (sentAt.get(view) ?? -Infinity) < 10_000) return;
		sentAt.set(view, now);
		(shared?.post ?? ((v: string) => api.post('/live/interest', { view: v })))(view).catch(() => {});
	}
	function interest(view: string) {
		if (!shared || status.leader) postInterest(view);
		else send({ t: 'interest', view });
	}

	function stop() {
		clearTimeout(retry);
		clearTimeout(hideTimer);
		paused = false;
		joined = false;
		withdraw();
		src?.close();
		src = null;
		chan?.close();
		chan = null;
		status.state = 'off';
	}

	function on(topic: string, fn: Listener): () => void {
		let set = listeners.get(topic);
		if (!set) listeners.set(topic, (set = new Set()));
		set.add(fn);
		return () => set.delete(fn);
	}

	return { status, start, stop, on, dispatch, interest };
}

// The one app-wide stream.
export const live = createLive(undefined, sharedParts());

// Close the stream when the module is replaced in dev.
if (import.meta.hot) import.meta.hot.dispose(() => live.stop());

// Cross-tab sharing needs the locks and broadcast APIs; without them each tab opens its own stream.
function sharedParts(): Shared | undefined {
	if (typeof navigator === 'undefined' || !navigator.locks || typeof BroadcastChannel === 'undefined') return undefined;
	return { locks: navigator.locks as unknown as LocksLike, channel: () => new BroadcastChannel(LOCK) as unknown as ChannelLike };
}
