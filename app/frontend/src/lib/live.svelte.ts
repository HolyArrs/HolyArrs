// Live updates: one event stream per browser (/live). With locks and
// BroadcastChannel the tabs elect one leader (navigator.locks): only it opens
// the stream and relays every message to the other tabs; when it closes or
// goes to the background, the next tab takes over and resumes with ?last=.
// Without them each tab has its own stream, as before.
// Each message is {"topic": "...", "data": ...}; pages subscribe to the topics they show.
// "resync" means the stream lost messages, so every listener reloads.
// Event ids are "<boot>-<n>": a new stream resumes with ?last=<id> and the
// server replays what was missed, or sends "resync" when it cannot (it
// restarted, or the gap is too old). A new boot seen here also reloads.
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

/** The parts of navigator.locks and BroadcastChannel the shared stream uses (fakes in tests). */
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

const LOCK = 'holyarrs-live';

import { backoff } from './loader.ts';

export function createLive(factory: SourceFactory = (u) => new EventSource(u) as unknown as SourceLike, shared?: Shared) {
	const status = $state<{ state: LiveState; last: number; leader: boolean }>({ state: 'off', last: 0, leader: false });
	const listeners = new Map<string, Set<Listener>>();
	let src: SourceLike | null = null;
	let lastId = '';

	/** seen keeps the newest event id; a new boot id means the server restarted. */
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
	// With a shared stream the background leader hands the stream to a tab in front.
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
			// A tab opened in the background never sees a change: start its countdown now.
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
			// Lost the server (restart, update): retry after 1 s, 2 s, 4 s… up to 15 s.
			// Without an id to resume from, the reopened stream cannot replay.
			// The server also ends a stream on purpose when the sign-in behind it
			// changed (a confirmed password, new rights): a live stream reopens at
			// once, quietly, and shows Reconnecting only if that fails too.
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

	// ---- Shared stream (one per browser) ----
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
			// Back from the background as a follower: an id that differs from the
			// leader's means messages passed while no tab held the stream.
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
	/** lead waits for the lock; holding it, this tab opens the stream and relays. */
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
	/** withdraw lets go of the lock (leader) or leaves the queue (follower). */
	function withdraw() {
		abort?.abort();
		abort = null;
		release?.();
	}
	const sentAt = new Map<string, number>();
	function postInterest(view: string) {
		const now = Date.now();
		if (now - (sentAt.get(view) ?? -Infinity) < 10_000) return;
		sentAt.set(view, now);
		(shared?.post ?? ((v: string) => api.post('/live/interest', { view: v })))(view).catch(() => {});
	}
	/** interest tells the server someone watches a view; only the leader posts it. */
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

	/** on listens to a topic ("*" for all) and returns the way to stop. */
	function on(topic: string, fn: Listener): () => void {
		let set = listeners.get(topic);
		if (!set) listeners.set(topic, (set = new Set()));
		set.add(fn);
		return () => set.delete(fn);
	}

	return { status, start, stop, on, dispatch, interest };
}

/** sharedParts is the real locks and channel, or undefined where the browser lacks either (each tab then has its own stream). */
function sharedParts(): Shared | undefined {
	if (typeof navigator === 'undefined' || !navigator.locks || typeof BroadcastChannel === 'undefined') return undefined;
	return { locks: navigator.locks as unknown as LocksLike, channel: () => new BroadcastChannel(LOCK) as unknown as ChannelLike };
}

export const live = createLive(undefined, sharedParts());

// Development: a hot update makes a new stream; the old one closes so open tabs do not pile up connections.
if (import.meta.hot) import.meta.hot.dispose(() => live.stop());
