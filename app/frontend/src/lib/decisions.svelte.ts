// The Needs a decision badge on Downloads: how many movies wait for a
// person. Asked again on page changes, on Slot news and every minute.

import { api } from './api.ts';
import type { Decision } from './downloaders.ts';

export const decisions = $state<{ count: number; items: Decision[] }>({ count: 0, items: [] });

/** refreshDecisions reads the list; errors leave the last one. */
export async function refreshDecisions(): Promise<void> {
	try {
		const r = await api.get<{ items: Decision[]; count: number }>('/needs-decision');
		decisions.items = r.items;
		decisions.count = r.count;
	} catch {
		// keep the last list
	}
}
