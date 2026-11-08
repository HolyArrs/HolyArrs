// Genre pages.

import { type ExploreRow, type ProviderItem, cardIdentity } from '#lib/ui/title-cards.ts';
import { type ExploreType } from './explore-type.ts';
import { type ServerRow } from './explore-server.ts';

export interface Genre {
	id: number;
	name: string;
}

export interface GenreAnswer {
	type: ExploreType;
	genre: { id: number; name: string; kind: 'movie' | 'show' };
	hero: ProviderItem[];
	rows: ServerRow[];
	/** Some lists were still out; the rest come on a later ask. */
	partial?: boolean;
}

/** TMDB's genre lists per kind, with its own fixed ids. */
export const GENRES: Record<'movie' | 'show', Genre[]> = {
	movie: [
		[28, 'Action'], [12, 'Adventure'], [16, 'Animation'], [35, 'Comedy'], [80, 'Crime'], [99, 'Documentary'], [18, 'Drama'], [10751, 'Family'],
		[14, 'Fantasy'], [36, 'History'], [27, 'Horror'], [10402, 'Music'], [9648, 'Mystery'], [10749, 'Romance'], [878, 'Science Fiction'],
		[53, 'Thriller'], [10752, 'War'], [37, 'Western']
	].map(([id, name]) => ({ id: id as number, name: name as string })),
	show: [
		[10759, 'Action & Adventure'], [16, 'Animation'], [35, 'Comedy'], [80, 'Crime'], [99, 'Documentary'], [18, 'Drama'], [10751, 'Family'],
		[10762, 'Kids'], [9648, 'Mystery'], [10764, 'Reality'], [10765, 'Sci-Fi & Fantasy'], [10768, 'War & Politics'], [37, 'Western']
	].map(([id, name]) => ({ id: id as number, name: name as string }))
};

/** The picker's genres for a tab (none under All and Music). */
export function genresFor(t: ExploreType): Genre[] {
	return t === 'movies' ? GENRES.movie : t === 'shows' ? GENRES.show : [];
}

/** Movie genres that TV combines with another (TMDB's TV genres are combined); under All their page holds movies and shows. */
const MERGED_GENRES = new Set([878, 14, 28, 12, 10752]);

/** Reads ?genre= for a tab: a known genre of that type, else null. Under All only a merged genre (movie id) counts. */
export function parseGenre(v: string | null | undefined, t: ExploreType): Genre | null {
	const id = Number(v);
	if (t === 'all') return MERGED_GENRES.has(id) ? (GENRES.movie.find((g) => g.id === id) ?? null) : null;
	return genresFor(t).find((g) => g.id === id) ?? null;
}

export function genrePath(t: ExploreType, genre: number): string {
	return `/explore/genre?${new URLSearchParams({ type: t, genre: String(genre) }).toString()}`;
}

/** Shows a title once in the first n rows with cards, counting the titles already shown above them (`taken`: the hero and Trending on your services). A row never drops under `min` cards for it; past that the repeats stay. */
export function dedupeTop<R extends ExploreRow>(rows: R[], taken: Iterable<string> = [], n = 8, min = 6): R[] {
	const seen = new Set(taken);
	let at = 0;
	return rows.map((r) => {
		if (at >= n || r.cards.length === 0) return r;
		at++;
		const fresh = r.cards.filter((c) => !seen.has(cardIdentity(c)));
		let cards = fresh;
		if (fresh.length < Math.min(min, r.cards.length)) {
			const room = Math.min(min, r.cards.length) - fresh.length;
			const back = new Set(r.cards.filter((c) => seen.has(cardIdentity(c))).slice(0, room));
			cards = r.cards.filter((c) => !seen.has(cardIdentity(c)) || back.has(c));
		}
		for (const c of cards) seen.add(cardIdentity(c));
		return cards.length === r.cards.length ? r : { ...r, cards };
	});
}
