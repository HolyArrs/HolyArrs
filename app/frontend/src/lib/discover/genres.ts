// Genre pages.

import { type ExploreRow, type ProviderItem, cardIdentity } from '#lib/ui/title-cards.ts';
import { type ExploreType } from './explore-type.ts';
import { type ServerRow } from './explore-server.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';

export interface Genre {
	id: number;
	/** The name in the person's language. */
	name: string;
	/** The English name, which art lookups and colours key on. */
	en?: string;
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
const genre = (id: number, en: string, name: () => string): Genre => ({ id, en, get name() { return name(); } });
export const GENRES: Record<'movie' | 'show', Genre[]> = {
	movie: [
		genre(28, 'Action', () => t('discover.genres.action')), genre(12, 'Adventure', () => t('discover.genres.adventure')), genre(16, 'Animation', () => t('discover.genres.animation')),
		genre(35, 'Comedy', () => t('discover.genres.comedy')), genre(80, 'Crime', () => t('discover.genres.crime')), genre(99, 'Documentary', () => t('discover.genres.documentary')),
		genre(18, 'Drama', () => t('discover.genres.drama')), genre(10751, 'Family', () => t('discover.genres.family')), genre(14, 'Fantasy', () => t('discover.genres.fantasy')),
		genre(36, 'History', () => t('discover.genres.history')), genre(27, 'Horror', () => t('discover.genres.horror')), genre(10402, 'Music', () => t('discover.genres.music')),
		genre(9648, 'Mystery', () => t('discover.genres.mystery')), genre(10749, 'Romance', () => t('discover.genres.romance')), genre(878, 'Science Fiction', () => t('discover.genres.scienceFiction')),
		genre(53, 'Thriller', () => t('discover.genres.thriller')), genre(10752, 'War', () => t('discover.genres.war')), genre(37, 'Western', () => t('discover.genres.western'))
	],
	show: [
		genre(10759, 'Action & Adventure', () => t('discover.genres.actionAdventure')), genre(16, 'Animation', () => t('discover.genres.animation')), genre(35, 'Comedy', () => t('discover.genres.comedy')),
		genre(80, 'Crime', () => t('discover.genres.crime')), genre(99, 'Documentary', () => t('discover.genres.documentary')), genre(18, 'Drama', () => t('discover.genres.drama')),
		genre(10751, 'Family', () => t('discover.genres.family')), genre(10762, 'Kids', () => t('discover.genres.kids')), genre(9648, 'Mystery', () => t('discover.genres.mystery')),
		genre(10764, 'Reality', () => t('discover.genres.reality')), genre(10765, 'Sci-Fi & Fantasy', () => t('discover.genres.sciFiFantasy')), genre(10768, 'War & Politics', () => t('discover.genres.warPolitics')),
		genre(37, 'Western', () => t('discover.genres.western'))
	]
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
