// The clean-up list of titles the chosen people finished watching.

import { t } from '#lib/core/i18n/i18n.svelte.ts';
import { bytes } from '#lib/core/format.ts';

/** A title every chosen person finished, from GET /library/watched-cleanup. */
export interface WatchedTitle {
	id: number;
	kind: 'movie' | 'show';
	name: string;
	year?: number;
	bytes: number;
	last_watched_at: string;
}

/** One account on a media server the list can wait for. */
export interface CleanupPerson {
	key: string;
	name: string;
	server: string;
	chosen: boolean;
}

export interface WatchedCleanup {
	enabled: boolean;
	days: number;
	people: CleanupPerson[];
	found: WatchedTitle[];
}

/** The query for the list: no people= when everyone is picked; '' when nobody is. */
export function cleanupQuery(days: number, picked: string[], people: CleanupPerson[]): string {
	if (!picked.length) return '';
	const q = new URLSearchParams({ days: String(days) });
	if (picked.length < people.length) q.set('people', picked.join(','));
	return '?' + q.toString();
}

/** The one line above the list: how many titles and how much room they use. */
export function watchedLine(c: WatchedCleanup | null): string {
	if (!c) return '';
	if (!c.found.length) return t('library.stats.cleanupNothing', { n: c.days });
	const size = c.found.reduce((n, f) => n + f.bytes, 0);
	return t('library.stats.cleanupFound', { n: c.found.length, size: bytes(size) });
}
