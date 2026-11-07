
export type FreshKind = 'new_episode' | 'new_season' | 'recently_added' | 'coming' | 'season_coming' | 'new_release';

export interface Fresh {
	kind: FreshKind;
	text: string;
	rank?: number;
}

export const FRESH_ORDER: readonly FreshKind[] = ['new_episode', 'new_season', 'recently_added', 'coming', 'season_coming', 'new_release'];

const MUSIC: readonly FreshKind[] = ['recently_added', 'new_release'];

export const ROW_SUPPRESS: Readonly<Record<string, readonly FreshKind[]>> = {
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

export function shownFresh(f: Fresh | null | undefined, where: { music?: boolean; row?: string } = {}): Fresh | null {
	if (!f || !FRESH_ORDER.includes(f.kind) || !f.text) return null;
	if (where.music && !MUSIC.includes(f.kind)) return null;
	if (where.row && ROW_SUPPRESS[where.row]?.includes(f.kind)) return null;
	return f;
}

const TYPE_WORD = { movie: 'Movie', show: 'Show' } as const;

export function typeWord(kind: string | undefined): string {
	return kind === 'movie' || kind === 'show' ? TYPE_WORD[kind] : '';
}

export function metaLine(kind: string | undefined, rest: readonly (string | number | null | undefined)[]): string[] {
	return [typeWord(kind), ...rest.map((r) => (r == null ? '' : String(r)))].filter(Boolean).filter((v, i, a) => a.indexOf(v) === i);
}

export function freshLabel(f: Fresh): string {
	return f.text;
}

export function pickFresh(list: readonly (Fresh | null | undefined)[]): Fresh | null {
	let best: Fresh | null = null;
	for (const f of list) if (f && (!best || FRESH_ORDER.indexOf(f.kind) < FRESH_ORDER.indexOf(best.kind))) best = f;
	return best;
}

export type OwnState = 'available' | 'partial' | 'downloading' | 'requested' | 'declined';

export function cardBadges(o: { fresh?: Fresh | null; owned?: { state: OwnState } | null; music?: boolean; row?: string }) {
	return {
		fresh: shownFresh(o.fresh, o),
		owned: o.owned ?? null
	};
}
