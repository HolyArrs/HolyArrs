// Plex watchlist sync: words for the account card and the small "from a Plex watchlist" line on asks.

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

const stateWords: Record<string, string> = {
	asked: 'Asked for',
	joined: 'Joined an ask',
	already: 'You asked before',
	in_library: 'In the Library',
	skipped: 'Skipped',
	limit: 'Waits for your weekly limit',
	failed: 'Tried again soon',
	no_match: 'Not found'
};

/** What came of one watchlist entry, in plain words. */
export function watchlistStateWord(state: string): string {
	return stateWords[state] ?? state;
}

/** The card shows once the owner allows it, a Plex account is linked and the person may ask. */
export function showWatchlistCard(v: PlexWatchlist | null): boolean {
	return !!v && v.on && v.linked && v.may_ask;
}

/** "From your Plex watchlist" (or someone's) for an ask made from one; "" otherwise. */
export function watchlistLine(people: Asker[] | undefined, me: number | undefined): string {
	const from = (people ?? []).filter((p) => p.from_watchlist);
	if (!from.length) return '';
	if (me !== undefined && from.some((p) => p.id === me)) return 'From your Plex watchlist';
	if (from.length > 1) return `From ${from.length} people’s Plex watchlists`;
	return `From ${from[0].name}’s Plex watchlist`;
}
