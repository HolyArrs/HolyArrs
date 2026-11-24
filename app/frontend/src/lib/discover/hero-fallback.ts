import type { ExploreCard, ExploreRow } from '../ui/title-cards.ts';

/** Stand-in hero titles while the server's own hero is not in yet (a slow server on a fresh install): the first movies and shows with a picture from the rows already loaded, so the hero is never an empty box. The server's hero replaces them as soon as it arrives. */
export function heroFallback(rows: readonly ExploreRow[], max = 3): ExploreCard[] {
	const out: ExploreCard[] = [];
	const seen = new Set<string>();
	for (const r of rows) {
		for (const c of r.cards) {
			if (out.length >= max) return out;
			if ((c.kind !== 'movie' && c.kind !== 'show') || !(c.wide || c.image) || seen.has(c.key)) continue;
			seen.add(c.key);
			out.push(c);
		}
	}
	return out;
}
