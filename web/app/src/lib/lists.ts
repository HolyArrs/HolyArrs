// Outside list helpers, kept apart from the page so they can be tested.

export const listKinds: [string, string, string][] = [
	['tmdb_list', 'TMDB list', 'The list number or its link, for example themoviedb.org/list/12345.'],
	['trakt_list', 'Trakt list', 'user/list, user/watchlist or the link, for example trakt.tv/users/ana/lists/favourites.']
];

/** listLink is where a list can be seen on its own site. */
export function listLink(kind: string, ref: string): string {
	if (kind === 'tmdb_list') return `https://www.themoviedb.org/list/${encodeURIComponent(ref)}`;
	const [user, slug] = ref.split('/');
	if (!user || !slug) return '';
	const u = encodeURIComponent(user);
	return slug === 'watchlist' ? `https://trakt.tv/users/${u}/watchlist` : `https://trakt.tv/users/${u}/lists/${encodeURIComponent(slug)}`;
}

/** every says how often a list is read, in words. */
export function every(hours: number): string {
	if (hours % 24 === 0) return hours === 24 ? 'every day' : `every ${hours / 24} days`;
	return `every ${hours} hours`;
}
