// Batches in-view title ids to the server so a later hover finds the trailer already resolved.

export interface PrefetchTimers {
	set: (f: () => void, ms: number) => ReturnType<typeof setTimeout>;
	clear: (t: ReturnType<typeof setTimeout>) => void;
}

export interface PrefetchQueueOptions {
	send: (ids: string[]) => Promise<string[]>;
	gatherMs?: number;
	max?: number;
	retryMs?: number;
	timers?: PrefetchTimers;
}

const realTimers: PrefetchTimers = { set: (f, ms) => setTimeout(f, ms), clear: (t) => clearTimeout(t) };

// Each id is sent once, and a still-pending one once more later; a full batch goes at once.
export function prefetchQueue(o: PrefetchQueueOptions) {
	const gather = o.gatherMs ?? 150;
	const max = o.max ?? 40;
	const retry = o.retryMs ?? 8000;
	const timers = o.timers ?? realTimers;
	const asked = new Map<string, number>(); // times sent per id
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
					// a failed prefetch is harmless: hover looks it up itself
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
		add: (id: string) => queue(id),
		flush,
		get waiting() {
			return waiting.length;
		}
	};
}
