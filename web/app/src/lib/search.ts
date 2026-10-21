export type SearchKind = 'movie' | 'show' | 'music';

/** searchAddress writes the controls' current values, even while page state is catching up. */
export function searchAddress(current: Pick<URL, 'href'>, kind: SearchKind, term: string): URL {
	const url = new URL(current.href);
	const query = term.trim();
	if (query) url.searchParams.set('q', query);
	else url.searchParams.delete('q');
	if (kind === 'movie') url.searchParams.delete('kind');
	else url.searchParams.set('kind', kind);
	return url;
}

export type KindPart = 'movies' | 'shows' | 'music';

/** kindOptions are the search filter's choices, leaving out kinds whose part is off. */
export function kindOptions(on: (part: KindPart) => boolean): { value: SearchKind; label: string; part: KindPart }[] {
	const all: { value: SearchKind; label: string; part: KindPart }[] = [
		{ value: 'movie', label: 'Movies', part: 'movies' },
		{ value: 'show', label: 'TV shows', part: 'shows' },
		{ value: 'music', label: 'Music', part: 'music' }
	];
	return all.filter((o) => on(o.part));
}

// ── The Search page (Explore card system) ────────────────────────────────
export type SearchScope = 'all' | 'show' | 'movie' | 'music' | 'people';
const scopes: SearchScope[] = ['all', 'people', 'movie', 'show', 'music'];

/** readScope reads ?kind=; anything unknown is All. */
export function readScope(v: string | null): SearchScope {
	return scopes.includes(v as SearchScope) ? (v as SearchScope) : 'all';
}

/** scopeAddress writes ?q= and ?kind= (left out for All). */
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

/** addRecent puts q first, drops a same-words duplicate and keeps RECENT_MAX. */
export function addRecent(list: string[], q: string): string[] {
	const t = q.trim();
	if (!t) return list;
	return [t, ...list.filter((x) => x.toLowerCase() !== t.toLowerCase())].slice(0, RECENT_MAX);
}

/** readRecent parses a stored list, tolerating anything broken. */
export function readRecent(raw: string | null): string[] {
	try {
		const v = JSON.parse(raw ?? '[]');
		return Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string').slice(0, RECENT_MAX) : [];
	} catch {
		return [];
	}
}

/** topMatches takes the first hits of each group in turn (the lookups answer best first). */
export function topMatches<T>(groups: T[][], max = 6): T[] {
	const out: T[] = [];
	for (let i = 0; out.length < max && groups.some((g) => i < g.length); i++) {
		for (const g of groups) if (i < g.length && out.length < max) out.push(g[i]);
	}
	return out;
}
