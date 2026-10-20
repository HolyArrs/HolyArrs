// Card badges: one freshness strip (from the server's `fresh`), one library
// status circle, and the type pill under All. Everything here is pure so the
// cards stay the same everywhere.

export type FreshKind = 'new_episode' | 'new_season' | 'recently_added' | 'top10' | 'coming' | 'season_coming' | 'new_release';

export interface Fresh {
	kind: FreshKind;
	text: string;
	rank?: number;
}

export const FRESH_ORDER: readonly FreshKind[] = ['new_episode', 'new_season', 'recently_added', 'top10', 'coming', 'season_coming', 'new_release'];

const MUSIC: readonly FreshKind[] = ['recently_added', 'new_release'];

/**
 * shownFresh is the strip a card shows, or null: music keeps Recently Added
 * and New Release only; the Top 10 row already shows its rank, so its own
 * Top 10 tag is dropped there.
 */
export function shownFresh(f: Fresh | null | undefined, where: { music?: boolean; top10Row?: boolean } = {}): Fresh | null {
	if (!f || !FRESH_ORDER.includes(f.kind) || !f.text) return null;
	if (where.music && !MUSIC.includes(f.kind)) return null;
	if (where.top10Row && f.kind === 'top10') return null;
	return f;
}

/** freshLabel is the strip's accessible name. */
export function freshLabel(f: Fresh): string {
	if (f.kind === 'top10' && f.rank) return `Number ${f.rank} in this week's Top 10`;
	return f.text;
}

/** pickFresh takes the highest-priority strip from several candidates. */
export function pickFresh(list: readonly (Fresh | null | undefined)[]): Fresh | null {
	let best: Fresh | null = null;
	for (const f of list) if (f && (!best || FRESH_ORDER.indexOf(f.kind) < FRESH_ORDER.indexOf(best.kind))) best = f;
	return best;
}

export type OwnState = 'available' | 'partial' | 'downloading' | 'requested' | 'declined';

/** cardBadges caps a card at one strip and one status circle (plus the type pill under All). */
export function cardBadges(o: { fresh?: Fresh | null; owned?: { state: OwnState } | null; typePill?: string; music?: boolean; top10Row?: boolean }) {
	return {
		fresh: shownFresh(o.fresh, o),
		owned: o.owned ?? null,
		typePill: o.typePill ?? null
	};
}
