// What an ask is for, in the plain words a request list uses: "Seasons" with a span joined by an en dash, "Season 3", "All seasons"; "3 albums", "Currents", "All albums". A movie needs no words beyond its type.
import { t } from '#lib/core/i18n/i18n.svelte.ts';
import type { Ask } from '../core/types.ts';

/** What an ask is for; `total` adds "All 4 seasons" or "All 9 albums" when known. */
export function askedFor(a: Pick<Ask, 'kind' | 'seasons' | 'albums'>, total: { seasons?: number; albums?: number } = {}): string {
	if (a.kind === 'show') {
		const s = [...new Set(a.seasons ?? [])].sort((x, y) => x - y);
		if (!s.length) return total.seasons ? t('requests.askedFor.allSeasonsN', { n: total.seasons }) : t('requests.askedFor.allSeasons');
		const specials = s[0] === 0;
		const rest = specials ? s.slice(1) : s;
		const words = rest.length === 0 ? '' : rest.length === 1 ? t('requests.askedFor.season', { n: rest[0] }) : t('requests.askedFor.seasons', { runs: runs(rest) });
		return specials ? (words ? t('requests.askedFor.specialsAnd', { words }) : t('requests.askedFor.specials')) : words;
	}
	if (a.kind === 'artist') {
		const al = a.albums ?? [];
		if (!al.length) return total.albums ? t('requests.askedFor.allAlbumsN', { n: total.albums }) : t('requests.askedFor.allAlbums');
		return al.length === 1 ? al[0].name : t('requests.askedFor.albums', { n: al.length });
	}
	return '';
}

/** Joins numbers into ranges: [1, 2, 4] becomes the span 1 to 2 (en dash), then 4. */
function runs(ns: number[]): string {
	const out: string[] = [];
	for (let i = 0; i < ns.length; i++) {
		let j = i;
		while (j + 1 < ns.length && ns[j + 1] === ns[j] + 1) j++;
		out.push(j > i ? `${ns[i]}–${ns[j]}` : String(ns[i]));
		i = j;
	}
	return out.join(', ');
}
