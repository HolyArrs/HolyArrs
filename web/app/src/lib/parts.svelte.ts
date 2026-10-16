// Which parts are on, asked again on page changes and every minute, so a
// part turned off hides its pages from the menu and its direct links.

import { api } from './api.ts';
import type { PartsAnswer } from './parts.ts';

export const partsNow = $state<{ answer: PartsAnswer | null }>({ answer: null });

/** refreshParts reads the parts; an error keeps what was known. */
export async function refreshParts(): Promise<void> {
	try {
		partsNow.answer = await api.get<PartsAnswer>('/parts');
	} catch {
		// keep the last answer
	}
}
