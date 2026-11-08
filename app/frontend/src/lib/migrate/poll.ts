
export interface FollowOptions<T> {
	get: (id: number, signal: AbortSignal) => Promise<T>;
	done: (answer: T) => boolean;
	onTick?: (answer: T) => void;
	onEnd?: (answer: T) => void;
	onLost?: (err: unknown, tries: number) => void;
	onBack?: () => void;
	every?: number;
	backoff?: number[];
}

export interface Follower {
	follow: (id: number) => void;
	stop: () => void;
	destroy: () => void;
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
