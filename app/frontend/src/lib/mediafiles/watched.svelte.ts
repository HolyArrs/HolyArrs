// Live copy of what the signed-in person watched: one request for the whole Library, shared by every page that marks titles.

import { api } from '../core/api.ts';
import { watchedIndex, type MyWatched, type WatchedIndex } from './watched.ts';

// Starts empty; nothing is marked until the answer arrives.
export const watchedNow = $state<{ ix: WatchedIndex }>({ ix: watchedIndex(null) });

// A failed fetch keeps the last answer.
export async function refreshWatched(): Promise<void> {
	try {
		watchedNow.ix = watchedIndex(await api.get<MyWatched>('/my/watched'));
	} catch {
	}
}
