// One follower for a running move or undo: one loop at a time, never after
// the page is gone, and a calm backoff when HolyArrs does not answer.

export interface FollowOptions<T> {
	/** get asks for the move once; the signal aborts it when the follow stops. */
	get: (id: number, signal: AbortSignal) => Promise<T>;
	/** done says the move (or undo) has finished. */
	done: (answer: T) => boolean;
	/** onTick hears every answer while it runs (and the last one). */
	onTick?: (answer: T) => void;
	/** onEnd hears the finished answer, once. */
	onEnd?: (answer: T) => void;
	/** onLost hears a failed ask (tries in a row so far). */
	onLost?: (err: unknown, tries: number) => void;
	/** onBack hears the first answer after one or more failed asks. */
	onBack?: () => void;
	/** Wait between asks, in ms (1s). */
	every?: number;
	/** Waits after failed asks, in ms (2s, 4s, then 10s). */
	backoff?: number[];
}

export interface Follower {
	/** follow starts following id; any earlier follow stops first. */
	follow: (id: number) => void;
	/** stop ends the follow (it can start again). */
	stop: () => void;
	/** destroy stops for good: nothing is asked or heard after it. */
	destroy: () => void;
	/** following is the id followed now, or null. */
	following: () => number | null;
}

export function createFollower<T>(o: FollowOptions<T>): Follower {
	const every = o.every ?? 1000;
	const backoff = o.backoff ?? [2000, 4000, 10000];
	let destroyed = false;
	let run = 0;
	let current: number | null = null;
	let timer: ReturnType<typeof setTimeout> | null = null;
	let ask: AbortController | null = null;

	function halt() {
		run++;
		current = null;
		if (timer) clearTimeout(timer);
		timer = null;
		ask?.abort();
		ask = null;
	}

	function follow(id: number) {
		if (destroyed) return;
		halt();
		const mine = ++run;
		current = id;
		let lost = 0;
		const live = () => !destroyed && mine === run;
		const tick = async () => {
			timer = null;
			if (!live()) return;
			ask = new AbortController();
			let answer: T;
			try {
				answer = await o.get(id, ask.signal);
			} catch (err) {
				if (!live()) return;
				lost++;
				o.onLost?.(err, lost);
				timer = setTimeout(tick, backoff[Math.min(lost - 1, backoff.length - 1)]);
				return;
			}
			if (!live()) return;
			if (lost > 0) {
				lost = 0;
				o.onBack?.();
			}
			o.onTick?.(answer);
			if (!live()) return;
			if (o.done(answer)) {
				current = null;
				run++;
				o.onEnd?.(answer);
				return;
			}
			timer = setTimeout(tick, every);
		};
		void tick();
	}

	return {
		follow,
		stop: halt,
		destroy() {
			destroyed = true;
			halt();
		},
		following: () => current
	};
}
