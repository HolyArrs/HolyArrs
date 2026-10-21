// Why a title is shown: short reasons drawn from data the page already has
// (the server's freshness strip, this week's Top 10, the household's taste).
// Pure, so the Explore hero and any card can share it.
import type { Fresh } from './fresh.ts';

export type WhyKind = 'new_season' | 'top10' | 'taste';

export interface Why {
	kind: WhyKind;
	text: string;
}

export interface WhyTitle {
	kind: string;
	tmdb: string;
	genres?: readonly string[];
	fresh?: Fresh | null;
}

export interface WhyContext {
	/** This week's Top 10 per kind: kind:tmdb -> rank. */
	top10?: ReadonlyMap<string, number>;
	/** Where the Top 10 comes from, for the label ("Netflix"). */
	top10Source?: string;
	/** The household's taste genres, strongest first. */
	taste?: readonly string[];
}

/** tasteMatch: the title shares two of the household's taste genres (one when the taste names two or fewer). */
export function tasteMatch(genres: readonly string[] | undefined, taste: readonly string[] | undefined): boolean {
	if (!genres?.length || !taste?.length) return false;
	const hit = taste.filter((g) => genres.includes(g)).length;
	return hit >= Math.min(2, taste.length);
}

/** whyShown is up to max reasons, strongest first: New season, the Top 10 rank, then Picked for you. */
export function whyShown(t: WhyTitle, ctx: WhyContext = {}, max = 2): Why[] {
	const out: Why[] = [];
	if (t.kind === 'show' && t.fresh?.kind === 'new_season') out.push({ kind: 'new_season', text: 'New season' });
	const rank = ctx.top10?.get(`${t.kind}:${t.tmdb}`) ?? (t.fresh?.kind === 'top10' ? t.fresh.rank : undefined);
	if (rank && rank >= 1 && rank <= 10) {
		const where = t.kind === 'show' ? 'TV shows' : 'Movies';
		out.push({ kind: 'top10', text: `#${rank} in ${where}${ctx.top10Source ? ` on ${ctx.top10Source}` : ''}` });
	}
	if (tasteMatch(t.genres, ctx.taste)) out.push({ kind: 'taste', text: 'Picked for you' });
	return out.slice(0, Math.max(0, max));
}

/** top10Map reads kind:tmdb -> best rank from Top 10 answers. */
export function top10Map(lists: readonly ({ kind?: string; items?: readonly { kind?: string; tmdb_id?: string | number; rank: number }[] } | null | undefined)[]): Map<string, number> {
	const m = new Map<string, number>();
	for (const l of lists) {
		for (const it of l?.items ?? []) {
			const kind = it.kind ?? l?.kind;
			if (!kind || !it.tmdb_id) continue;
			const k = `${kind}:${it.tmdb_id}`;
			if (!m.has(k) || it.rank < (m.get(k) as number)) m.set(k, it.rank);
		}
	}
	return m;
}
