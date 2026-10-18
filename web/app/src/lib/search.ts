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
