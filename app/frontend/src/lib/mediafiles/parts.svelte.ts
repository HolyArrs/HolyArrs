// Live copy of which parts of the app are switched on, for hiding what is off.

import { api } from '../core/api.ts';
import type { PartsAnswer } from './parts.ts';

// Starts empty; nothing is hidden until the first answer arrives.
export const partsNow = $state<{ answer: PartsAnswer | null }>({ answer: null });

// A failed fetch keeps the last answer instead of hiding everything.
export async function refreshParts(): Promise<void> {
	try {
		partsNow.answer = await api.get<PartsAnswer>('/parts');
	} catch {
	}
}

// Counts as on until the answer says otherwise, so the UI does not flicker.
export const partOn = (name: string): boolean => !!partsNow.answer && find(name)?.on !== false;

export const partOff = (name: string): boolean => find(name)?.on === false;

// Looks up one part in the current answer.
const find = (name: string) => partsNow.answer?.parts.find((p) => p.part === name);
