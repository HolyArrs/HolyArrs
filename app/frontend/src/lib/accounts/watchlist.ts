// Plex watchlist sync: words for the account card and the small "from a Plex watchlist" line on asks.
import { t } from '#lib/core/i18n/i18n.svelte.ts';

export type WatchlistItem = {
	name: string;
	year?: number;
	kind: 'movie' | 'show';
	state: string;
	ask_id?: number;
	note?: string;
	seen_at: string;
};

export type PlexWatchlist = {
	on: boolean;
	everyone?: boolean;
	chosen?: boolean;
	linked: boolean;
	plex_name?: string;
	can_read: boolean;
	may_ask: boolean;
	movies: boolean;
	shows: boolean;
	checked_at?: string;
	error?: string;
	items: WatchlistItem[];
};

type Asker = { id: number; name: string; from_watchlist?: boolean };

const stateKeys = ['asked', 'joined', 'already', 'in_library', 'skipped', 'limit', 'failed', 'no_match'];

/** What came of one watchlist entry, in plain words. */
export function watchlistStateWord(state: string): string {
	return stateKeys.includes(state) ? t(`accounts.watchlist.state.${state}`) : state;
}

/** The card shows once the owner allows it, a Plex account is linked and the person may ask. */
export function showWatchlistCard(v: PlexWatchlist | null): boolean {
	return !!v && v.on && v.linked && v.may_ask;
}

/** "From your Plex watchlist" (or someone's) for an ask made from one; "" otherwise. */
export function watchlistLine(people: Asker[] | undefined, me: number | undefined): string {
	const from = (people ?? []).filter((p) => p.from_watchlist);
	if (!from.length) return '';
	if (me !== undefined && from.some((p) => p.id === me)) return t('accounts.watchlist.yours');
	if (from.length > 1) return t('accounts.watchlist.many', { n: from.length });
	return t('accounts.watchlist.one', { name: from[0].name });
}
