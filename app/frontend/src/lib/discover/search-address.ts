import { t } from '#lib/core/i18n/i18n.svelte.ts';
export type SearchScope = 'all' | 'show' | 'movie' | 'music' | 'people';

/** The slice of a Library title the "In your Library" row reads. */
export type LibraryHit = {
	kind: string;
	slots?: { state: string; download?: { active?: boolean } | null }[] | null;
	show?: { seasons: { number: number }[]; missing: number } | null;
	artist?: { albums: number; missing: number } | null;
};

const scopes: SearchScope[] = ['all', 'people', 'movie', 'show', 'music'];

/** Reads ?kind=; anything unknown is All. */
export function readScope(v: string | null): SearchScope {
	return scopes.includes(v as SearchScope) ? (v as SearchScope) : 'all';
}

/** Writes ?q= and ?kind= (left out for All). */
export function scopeAddress(current: Pick<URL, 'href'>, scope: SearchScope, term: string): URL {
	const url = new URL(current.href);
	const q = term.trim();
	if (q) url.searchParams.set('q', q);
	else url.searchParams.delete('q');
	if (scope === 'all') url.searchParams.delete('kind');
	else url.searchParams.set('kind', scope);
	return url;
}

export const RECENT_MAX = 8;
export const recentKey = (user: string | number | undefined) => `holyarrs.search.recent.${user ?? 'anon'}`;

/** Puts q first, drops a same-words duplicate and keeps RECENT_MAX. */
export function addRecent(list: string[], q: string): string[] {
	const t = q.trim();
	if (!t) return list;
	return [t, ...list.filter((x) => x.toLowerCase() !== t.toLowerCase())].slice(0, RECENT_MAX);
}

/** Parses a stored list, tolerating anything broken. */
export function readRecent(raw: string | null): string[] {
	try {
		const v = JSON.parse(raw ?? '[]');
		return Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string').slice(0, RECENT_MAX) : [];
	} catch {
		return [];
	}
}

/** Takes the first hits of each group in turn (the lookups answer best first). */
export function topMatches<T>(groups: T[][], max = 6): T[] {
	const out: T[] = [];
	for (let i = 0; out.length < max && groups.some((g) => i < g.length); i++) {
		for (const g of groups) if (i < g.length && out.length < max) out.push(g[i]);
	}
	return out;
}

/** Puts hits whose name is exactly the typed words first ("drake" gives Drake before Vera Drake), keeping the interleaved order otherwise. Exact hits deeper in a group are pulled up, one per group. */
export function rankTop<T extends { name: string }>(groups: T[][], term: string, max = 6): T[] {
	const want = term.trim().toLocaleLowerCase();
	const exact = (c: T) => !!want && c.name.trim().toLocaleLowerCase() === want;
	// One exact hit per group, so a dozen same-named bands cannot crowd out the rest.
	const first = groups.map((g) => g.find(exact)).filter((c): c is T => !!c);
	const rest = topMatches(groups.map((g) => g.filter((c) => !first.includes(c))), max);
	return [...first, ...rest].slice(0, max);
}

/** A Library card's state line in Search: "Downloading", "Missing 2 episodes", or what is kept ("4 seasons · up to date"). */
export function libraryNote(hit: LibraryHit, fallback = ''): string {
	if ((hit.slots ?? []).some((s) => s.download?.active || s.state === 'downloading' || s.state === 'importing')) return t('discover.searchAddress.downloading');
	if (hit.kind === 'show' && hit.show) {
		if (hit.show.missing > 0) return hit.show.missing === 1 ? t('discover.searchAddress.missingEpisodeOne') : t('discover.searchAddress.missingEpisodeOther', { n: hit.show.missing });
		const seasons = hit.show.seasons.filter((s) => s.number > 0).length;
		return seasons === 1 ? t('discover.searchAddress.seasonOneUpToDate') : t('discover.searchAddress.seasonOtherUpToDate', { n: seasons });
	}
	if (hit.kind === 'artist' && hit.artist) {
		if (hit.artist.missing > 0) return hit.artist.missing === 1 ? t('discover.searchAddress.missingTrackOne') : t('discover.searchAddress.missingTrackOther', { n: hit.artist.missing });
		return hit.artist.albums === 1 ? t('discover.searchAddress.albumOneUpToDate') : t('discover.searchAddress.albumOtherUpToDate', { n: hit.artist.albums });
	}
	return fallback;
}
