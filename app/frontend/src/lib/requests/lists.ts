// Outside list helpers, kept apart from the page so they can be tested.
import { t } from '#lib/core/i18n/i18n.svelte.ts';
import { seasonChoices } from '../shows/index.ts';

/** A tuple whose words are looked up each time they are read, so they follow the language picked. */
function words<T extends string[]>(...parts: (string | (() => string))[]): T {
	const out: string[] = [];
	parts.forEach((p, i) => Object.defineProperty(out, i, { enumerable: true, get: typeof p === 'function' ? p : () => p }));
	return out as T;
}

/** What a list adds. */
export const titleKinds: [string, string][] = [
	words<[string, string]>('movie', () => t('requests.lists.movies')),
	words<[string, string]>('show', () => t('requests.lists.shows'))
];

/** The season choices for a show list: the ones a show is added with by hand, plus the first season only. */
export const listSeasons: [string, string, string][] = [
	...seasonChoices.filter(([v]) => v !== 'none'),
	words<[string, string, string]>('first', () => t('requests.lists.firstSeason'), () => t('requests.lists.firstSeasonHint')),
	...seasonChoices.filter(([v]) => v === 'none')
];

export const listKinds: [string, string, string][] = [
	words<[string, string, string]>('tmdb_list', () => t('requests.lists.tmdb'), () => t('requests.lists.tmdbHint')),
	words<[string, string, string]>('trakt_list', () => t('requests.lists.trakt'), () => t('requests.lists.traktHint'))
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
	if (hours % 24 === 0) return hours === 24 ? t('requests.lists.everyDay') : t('requests.lists.everyDays', { n: hours / 24 });
	return t('requests.lists.everyHours', { n: hours });
}
