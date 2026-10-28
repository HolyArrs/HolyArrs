// A slow source makes the server answer an artist or album page after about
// 3 s with what it has and incomplete:true; it keeps looking. The page then
// asks again quietly (2.5 s, then a little longer each time, up to 4 times),
// merges each answer into what is on screen, and stops as soon as an answer
// is complete. Leaving the page stops it. Incomplete answers carry no-store
// and are never kept by api.get, so each ask reaches the server.
import { isIncomplete } from '../api.ts';

/** The pauses before each quiet re-ask. */
export const REASK_MS = [2500, 3500, 5000, 7000];

/** sleep waits ms, or rejects at once when signal aborts. */
export function sleep(ms: number, signal: AbortSignal): Promise<void> {
	return new Promise((resolve, reject) => {
		if (signal.aborted) return reject(signal.reason);
		const t = setTimeout(() => {
			signal.removeEventListener('abort', stop);
			resolve();
		}, ms);
		const stop = () => {
			clearTimeout(t);
			reject(signal.reason);
		};
		signal.addEventListener('abort', stop, { once: true });
	});
}

/**
 * reaskWhileIncomplete hands `first` to take, then while the answer is incomplete asks
 * again after each pause and hands every answer on. `pending` says another try is still
 * coming (a page shows its skeleton there, not "not known yet"). A failed re-ask keeps
 * what is shown. When `same` says an answer brought nothing new over the last one,
 * the source is stuck: asking stops and what is there is shown as final.
 * Answers how many re-asks it made.
 */
export async function reaskWhileIncomplete<T>(
	first: T,
	ask: () => Promise<T>,
	take: (v: T, pending: boolean) => void,
	signal: AbortSignal,
	waits: number[] = REASK_MS,
	wait: (ms: number, signal: AbortSignal) => Promise<void> = sleep,
	same?: (prev: T, next: T) => boolean
): Promise<number> {
	let last = first;
	let tries = 0;
	take(first, isIncomplete(first) && waits.length > 0);
	while (isIncomplete(last) && tries < waits.length) {
		try {
			await wait(waits[tries], signal);
		} catch {
			return tries;
		}
		tries++;
		let next: T;
		try {
			next = await ask();
		} catch {
			if (signal.aborted) return tries;
			if (tries >= waits.length) take(last, false);
			continue;
		}
		if (signal.aborted) return tries;
		const stuck = !!same && isIncomplete(next) && same(last, next);
		last = next;
		take(next, isIncomplete(next) && tries < waits.length && !stuck);
		if (stuck) return tries;
	}
	return tries;
}

/**
 * keepShown merges a newer answer over the one on screen: while the newer answer is
 * incomplete, a list it has shorter (still filling in) keeps the longer one already shown.
 * A complete answer replaces everything.
 */
export function keepShown<T extends object>(shown: T | null, next: T, lists: (keyof T)[]): T {
	if (!shown || !isIncomplete(next)) return next;
	const out = { ...next };
	for (const k of lists) {
		const a = shown[k];
		const b = next[k];
		if (Array.isArray(a) && (!Array.isArray(b) || b.length < a.length)) out[k] = a;
	}
	return out;
}

/** sameLists says two answers hold lists of the same lengths (nothing new arrived). */
export function sameLists<T extends object>(lists: (keyof T)[]): (a: T, b: T) => boolean {
	const n = (v: unknown) => (Array.isArray(v) ? v.length : -1);
	return (a, b) => lists.every((k) => n(a[k]) === n(b[k]));
}
