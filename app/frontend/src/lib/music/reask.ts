// A slow source makes the server answer an artist or album page after about 3 s with what it has and incomplete:true; it keeps looking. The page then asks again quietly (2.5 s, then a little longer each time, up to 4 times) and merges what comes in.
import { isFilling, isIncomplete } from '../core/api.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';

/** The pauses before each quiet re-ask. */
export const REASK_MS = [2500, 3500, 5000, 7000];

/** The pauses while the server still gets the item or holds it in line: every 2 s for about 3 minutes, then the page gives up. */
export const GAP_REASK_MS: number[] = Array.from({ length: 90 }, () => 2000);

/** The pauses to use for a first answer, the long ones when it says the server is still getting it. */
export const gapWaits = (first: { gap?: unknown } | null | undefined, normal: number[] = REASK_MS): number[] => (first?.gap ? GAP_REASK_MS : normal);

/** Waits ms, or rejects at once when the signal aborts. */
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

/** Hands `first` to `take`, then while the answer is incomplete asks again after each pause and hands every answer on. `pending` says another try is still coming (a page shows its skeleton there, not "not known"). */
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

/** Merges a newer answer over the one on screen. While the newer answer is incomplete, a list it has shorter (still filling in) keeps the longer one already shown. A complete answer replaces everything. */
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

/** Whether two answers hold lists of the same lengths (nothing new arrived). */
export function sameLists<T extends object>(lists: (keyof T)[]): (a: T, b: T) => boolean {
	const n = (v: unknown) => (Array.isArray(v) ? v.length : -1);
	return (a, b) => lists.every((k) => n(a[k]) === n(b[k]));
}

/** The pauses of the quiet re-asks for a song list: quick at first (the server fetches the list the moment the album or its artist opens, so a list ready at 9 s shows at about 9 s), then backing off; about 60 s in all. */
export const TRACK_REASK_MS = [1000, 1500, 2000, 2000, 2500, 3000, 4000, 5000, 6000, 8000, 8000, 8000, 8000];

/** What a song list shows: real rows, skeleton rows while songs are still being fetched, or the neutral line once asking has stopped. */
export function trackListView(count: number, pending: boolean): 'rows' | 'skeleton' | 'gave-up' {
	if (count > 0) return 'rows';
	return pending ? 'skeleton' : 'gave-up';
}

/** The neutral line shown after the list stayed empty for about a minute. */
export const NO_SONGS_LINE = 'Songs are not available right now.';

/** The same line in the person's language. */
export const noSongsLine = () => t('music.songsAreNotAvailable');

/** An answer that still holds no songs is never "stuck"; asking goes on until the pauses run out. */
export function sameTracks<T extends { tracks: unknown[] }>(a: T, b: T): boolean {
	return a.tracks.length > 0 && a.tracks.length === b.tracks.length;
}

/** The pauses of the quiet re-asks for an answer still filling in (Explore music, artist search). */
export const FILLING_REASK_MS = [1500, 2500, 4000, 6000, 9000];

/** Asks again after each pause while the last answer is still filling in ("partial" or "pending"), handing every newer answer to `take`. What is shown stays until a newer answer arrives, so nothing jumps. */
export async function reaskWhileFilling<T>(
	first: T,
	ask: () => Promise<T>,
	take: (v: T) => void,
	signal?: AbortSignal,
	waits: number[] = FILLING_REASK_MS,
	wait: (ms: number, signal: AbortSignal) => Promise<void> = sleep
): Promise<T> {
	const sig = signal ?? new AbortController().signal;
	let last = first;
	for (let i = 0; isFilling(last) && i < waits.length; i++) {
		try {
			await wait(waits[i], sig);
		} catch {
			return last;
		}
		try {
			const next = await ask();
			if (sig.aborted) return last;
			last = next;
			take(next);
		} catch {
			if (sig.aborted) return last;
		}
	}
	return last;
}
