// Which parts are on, asked again on page changes and every minute, so a
// part turned off hides its pages from the menu and its direct links.

import { api } from '../core/api.ts';
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

const find = (name: string) => partsNow.answer?.parts.find((p) => p.part === name);

/** partOn is true once the parts answer says the part is on; false until it loads. */
export const partOn = (name: string): boolean => !!partsNow.answer && find(name)?.on !== false;

/** partOff is true only once the parts answer says the part is off. */
export const partOff = (name: string): boolean => find(name)?.on === false;
