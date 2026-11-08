// The Needs a decision badge on Downloads: how many movies wait for a person. Read again on page changes, on Slot news and every minute.

import { api } from '../core/api.ts';
import type { Decision } from '../core/types.ts';

export const decisions = $state<{ count: number; items: Decision[] }>({ count: 0, items: [] });

/** Errors leave the last list. */
export async function refreshDecisions(): Promise<void> {
	try {
		const r = await api.get<{ items: Decision[]; count: number }>('/needs-decision');
		decisions.items = r.items;
		decisions.count = r.count;
	} catch {
		// Keep the last list.
	}
}
