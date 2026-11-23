// The requests list in pages: the server sends the newest 200 at a time (?limit=, ?offset=).
import type { Ask } from '../core/types.ts';

export type Counts = { pending: number; processing: number; done: number; failed: number };
export type ListPage = { asks: Ask[]; counts?: Counts; total?: number };
type Get = (path: string) => Promise<ListPage>;

export const PAGE = 200;

/** The list from the top, as far as `have` (what the page had loaded, after scrolling), so a re-read keeps the cards already shown. */
export async function readList(get: Get, have: number): Promise<ListPage> {
	const first = await get(`/asks?limit=${PAGE}`);
	const want = Math.min(have, 500 + PAGE);
	if (want <= PAGE || first.asks.length < PAGE) return first;
	const rest = await get(`/asks?limit=${want - PAGE}&offset=${PAGE}`);
	const seen = new Set(first.asks.map((a) => a.id));
	return { ...first, asks: [...first.asks, ...rest.asks.filter((a) => !seen.has(a.id))] };
}

/** The next page after the `have` cards loaded. */
export const readMore = (get: Get, have: number) => get(`/asks?limit=${PAGE}&offset=${have}`);

/** Every waiting request the filters show, read page by page: what Approve all takes and counts. */
export async function allWaiting(get: Get, match: (a: Ask) => boolean): Promise<Ask[]> {
	const out: Ask[] = [];
	const seen = new Set<number>();
	for (let off = 0; ; off += 500) {
		const r = await get(`/asks?state=asked&limit=500${off ? `&offset=${off}` : ''}`);
		const fresh = r.asks.filter((a) => !seen.has(a.id));
		for (const a of fresh) seen.add(a.id);
		// A server that ignores ?offset= sends the same page again: stop there.
		if (off && !fresh.length) return out;
		out.push(...fresh.filter((a) => a.state === 'asked' && match(a)));
		if (r.asks.length < 500) return out;
	}
}
