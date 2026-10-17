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
