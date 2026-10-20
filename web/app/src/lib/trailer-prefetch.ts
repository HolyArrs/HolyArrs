// The in-view trailer prefetch: title cards that stay on screen for a
// while are named to the server in one batch (GET /trailer/batch), so a
// later hover finds the address already resolved. Each title is asked at
// most once per page (a pending one once more, later); a full batch goes
// at once, a partial one after a short gather.

export interface PrefetchTimers {
	set: (f: () => void, ms: number) => ReturnType<typeof setTimeout>;
	clear: (t: ReturnType<typeof setTimeout>) => void;
}

export interface PrefetchQueueOptions {
	/** Sends one batch; answers the ids still pending (asked again once, after retryMs). */
	send: (ids: string[]) => Promise<string[]>;
	/** How long to gather ids before sending a partial batch. */
	gatherMs?: number;
	/** Most ids in one batch (the server allows 40). */
	max?: number;
	/** When pending ids are asked once more. */
	retryMs?: number;
	timers?: PrefetchTimers;
}

const realTimers: PrefetchTimers = { set: (f, ms) => setTimeout(f, ms), clear: (t) => clearTimeout(t) };

export function prefetchQueue(o: PrefetchQueueOptions) {
	const gather = o.gatherMs ?? 150;
	const max = o.max ?? 40;
	const retry = o.retryMs ?? 8000;
	const timers = o.timers ?? realTimers;
	const asked = new Map<string, number>(); // id -> times sent
	let waiting: string[] = [];
	let t: ReturnType<typeof setTimeout> | null = null;

	function flush() {
		if (t !== null) timers.clear(t);
		t = null;
		while (waiting.length) {
			const ids = waiting.splice(0, max);
			void o
				.send(ids)
				.then((pending) => {
					const again = pending.filter((id) => (asked.get(id) ?? 0) < 2);
					if (again.length) timers.set(() => again.forEach((id) => queue(id, true)), retry);
				})
				.catch(() => {
					/* the hover lookup still works */
				});
		}
	}

	function queue(id: string, isRetry = false) {
		const n = asked.get(id) ?? 0;
		if (isRetry ? n >= 2 : n >= 1) return;
		asked.set(id, n + 1);
		waiting.push(id);
		if (waiting.length >= max) flush();
		else if (t === null) t = timers.set(flush, gather);
	}

	return {
		/** Names one title ("movie:123"); repeats are ignored. */
		add: (id: string) => queue(id),
		flush,
		get waiting() {
			return waiting.length;
		}
	};
}
