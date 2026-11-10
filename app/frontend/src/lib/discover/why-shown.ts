// Why a title is shown: short reasons drawn from data the page already has (the server's freshness strip, the household's taste). Pure, so the Explore hero and any card can share it.
import type { Fresh } from '#lib/ui/fresh.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';

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
	/** The household's taste genres, strongest first. */
	taste?: readonly string[];
}

/** The title shares two of the household's taste genres (one when the taste names two or fewer). */
export function tasteMatch(genres: readonly string[] | undefined, taste: readonly string[] | undefined): boolean {
	if (!genres?.length || !taste?.length) return false;
	const hit = taste.filter((g) => genres.includes(g)).length;
	return hit >= Math.min(2, taste.length);
}

/** Up to `max` reasons, strongest first: New season, then Picked for you. */
export function whyShown(title: WhyTitle, ctx: WhyContext = {}, max = 2): Why[] {
	const out: Why[] = [];
	if (title.kind === 'show' && title.fresh?.kind === 'new_season') out.push({ kind: 'new_season', text: t('discover.whyShown.newSeason') });
	if (tasteMatch(title.genres, ctx.taste)) out.push({ kind: 'taste', text: t('discover.whyShown.pickedForYou') });
	return out.slice(0, Math.max(0, max));
}
