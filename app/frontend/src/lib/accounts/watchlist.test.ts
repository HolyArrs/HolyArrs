import { showWatchlistCard, watchlistLine, watchlistStateWord, type PlexWatchlist } from './watchlist.ts';
import { describe, expect, it } from 'vitest';

const base: PlexWatchlist = { on: true, linked: true, can_read: true, may_ask: true, movies: false, shows: false, items: [] };

describe('Plex watchlist', () => {
	it('shows the card only when allowed, linked and the person may ask', () => {
		expect(showWatchlistCard(base)).toBe(true);
		expect(showWatchlistCard({ ...base, on: false })).toBe(false);
		expect(showWatchlistCard({ ...base, linked: false })).toBe(false);
		expect(showWatchlistCard({ ...base, may_ask: false })).toBe(false);
		expect(showWatchlistCard(null)).toBe(false);
	});
	it('names entry states in plain words', () => {
		expect(watchlistStateWord('asked')).toBe('Asked for');
		expect(watchlistStateWord('limit')).toMatch(/weekly limit/);
		expect(watchlistStateWord('odd')).toBe('odd');
	});
	it('says whose watchlist an ask came from', () => {
		expect(watchlistLine([{ id: 2, name: 'ana' }], 2)).toBe('');
		expect(watchlistLine(undefined, 2)).toBe('');
		expect(watchlistLine([{ id: 2, name: 'ana', from_watchlist: true }], 2)).toBe('From your Plex watchlist');
		expect(watchlistLine([{ id: 2, name: 'ana', from_watchlist: true }], 3)).toBe('From ana’s Plex watchlist');
		expect(watchlistLine([{ id: 2, name: 'ana', from_watchlist: true }, { id: 4, name: 'bo', from_watchlist: true }], 3)).toBe('From 2 people’s Plex watchlists');
	});
});
