
// Freshness tags on cards and which rows already say the same thing.

import { t } from '#lib/core/i18n/i18n.svelte.ts';
import { serverWords } from '#lib/core/i18n/server-words.ts';
export type FreshKind = 'new_episode' | 'new_season' | 'recently_added' | 'coming' | 'season_coming' | 'new_release';

export interface Fresh {
	kind: FreshKind;
	text: string;
	rank?: number;
}

export type OwnState = 'available' | 'partial' | 'downloading' | 'requested' | 'declined';

// Most urgent first; pickFresh keeps the earliest.
export const FRESH_ORDER: readonly FreshKind[] = ['new_episode', 'new_season', 'recently_added', 'coming', 'season_coming', 'new_release'];

// Only these make sense on music cards.
const MUSIC: readonly FreshKind[] = ['recently_added', 'new_release'];

// A row's own title already says this, so the tag would repeat it.
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

// Drops tags that are unknown, empty, wrong for music, or redundant in the row.
export function shownFresh(f: Fresh | null | undefined, where: { music?: boolean; row?: string } = {}): Fresh | null {
	if (!f || !FRESH_ORDER.includes(f.kind) || !f.text) return null;
	if (where.music && !MUSIC.includes(f.kind)) return null;
	if (where.row && ROW_SUPPRESS[where.row]?.includes(f.kind)) return null;
	return f;
}

export function typeWord(kind: string | undefined): string {
	return kind === 'movie' ? t('ui.fresh.movie') : kind === 'show' ? t('ui.fresh.show') : '';
}

// Joins type and details, skipping blanks and repeats.
export function metaLine(kind: string | undefined, rest: readonly (string | number | null | undefined)[]): string[] {
	return [typeWord(kind), ...rest.map((r) => (r == null ? '' : String(r)))].filter(Boolean).filter((v, i, a) => a.indexOf(v) === i);
}

export function freshLabel(f: Fresh): string {
	return serverWords(f.text);
}

export function pickFresh(list: readonly (Fresh | null | undefined)[]): Fresh | null {
	let best: Fresh | null = null;
	for (const f of list) if (f && (!best || FRESH_ORDER.indexOf(f.kind) < FRESH_ORDER.indexOf(best.kind))) best = f;
	return best;
}

export function cardBadges(o: { fresh?: Fresh | null; owned?: { state: OwnState } | null; music?: boolean; row?: string }) {
	return {
		fresh: shownFresh(o.fresh, o),
		owned: o.owned ?? null
	};
}
