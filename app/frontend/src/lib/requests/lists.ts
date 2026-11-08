// Outside list helpers, kept apart from the page so they can be tested.
import { seasonChoices } from '../shows/index.ts';

/** What a list adds. */
export const titleKinds: [string, string][] = [
	['movie', 'Movies'],
	['show', 'Shows']
];

/** The season choices for a show list: the ones a show is added with by hand, plus the first season only. */
export const listSeasons: [string, string, string][] = [
	...seasonChoices.filter(([v]) => v !== 'none'),
	['first', 'The first season', 'Only season 1 (or the earliest season when a show has no season 1), to try a show out.'],
	...seasonChoices.filter(([v]) => v === 'none')
];

export const listKinds: [string, string, string][] = [
	['tmdb_list', 'TMDB list', 'The list number or its link, for example themoviedb.org/list/12345.'],
	['trakt_list', 'Trakt list', 'user/list, user/watchlist or the link, for example trakt.tv/users/ana/lists/favourites.']
];

/** Where a list can be seen on its own site. */
export function listLink(kind: string, ref: string): string {
	if (kind === 'tmdb_list') return `https://www.themoviedb.org/list/${encodeURIComponent(ref)}`;
	const [user, slug] = ref.split('/');
	if (!user || !slug) return '';
	const u = encodeURIComponent(user);
	return slug === 'watchlist' ? `https://trakt.tv/users/${u}/watchlist` : `https://trakt.tv/users/${u}/lists/${encodeURIComponent(slug)}`;
}

/** How often a list is read, in words. */
export function every(hours: number): string {
	if (hours % 24 === 0) return hours === 24 ? 'every day' : `every ${hours / 24} days`;
	return `every ${hours} hours`;
}
