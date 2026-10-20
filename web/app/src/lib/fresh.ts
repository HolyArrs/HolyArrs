// Card badges: one freshness badge (from the server's `fresh`: a bottom strip,
// or the small Top 10 corner tag) and one library status circle. The type is
// a word on the metadata line, never a badge. Everything here is pure so the
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
 * ROW_SUPPRESS mirrors the server's explore.RowSuppress: a card never
 * repeats what its row already says. The server drops these first; the page
 * checks again so a cached answer never shows one.
 */
export const ROW_SUPPRESS: Readonly<Record<string, readonly FreshKind[]>> = {
	top10: ['top10'],
	best: ['new_release'],
	new_services: ['new_release'],
	fresh: ['new_release'],
	new_from: ['new_release'],
	just_arrived: ['recently_added'],
	recent: ['recently_added'],
	coming_soon: ['coming', 'season_coming'],
	upcoming: ['coming', 'season_coming'],
	new_episodes: ['new_episode'],
	new_seasons: ['new_season']
};

/**
 * shownFresh is the one badge a card shows, or null: music keeps Recently
 * Added and New Release only, and a row never repeats its own meaning (the
 * Top 10 row drops the Top 10 tag, Just arrived drops Recently Added...).
 */
export function shownFresh(f: Fresh | null | undefined, where: { music?: boolean; top10Row?: boolean; row?: string } = {}): Fresh | null {
	if (!f || !FRESH_ORDER.includes(f.kind) || !f.text) return null;
	if (where.music && !MUSIC.includes(f.kind)) return null;
	if (where.top10Row && f.kind === 'top10') return null;
	if (where.row && ROW_SUPPRESS[where.row]?.includes(f.kind)) return null;
	return f;
}

const TYPE_WORD = { movie: 'Movie', show: 'Show' } as const;

/** typeWord is the first word of a card's metadata line ("Show"), or '' for other kinds. */
export function typeWord(kind: string | undefined): string {
	return kind === 'movie' || kind === 'show' ? TYPE_WORD[kind] : '';
}

/** metaLine puts the type first: "Show · 4 Seasons · 2024", "Movie · 2h 10m · 2024". */
export function metaLine(kind: string | undefined, rest: readonly (string | number | null | undefined)[]): string[] {
	return [typeWord(kind), ...rest.map((r) => (r == null ? '' : String(r)))].filter(Boolean).filter((v, i, a) => a.indexOf(v) === i);
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

/** cardBadges caps a card at one freshness badge and one status circle; there is no type badge. */
export function cardBadges(o: { fresh?: Fresh | null; owned?: { state: OwnState } | null; music?: boolean; top10Row?: boolean; row?: string }) {
	return {
		fresh: shownFresh(o.fresh, o),
		owned: o.owned ?? null
	};
}
