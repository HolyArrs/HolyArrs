
import { api } from '../core/api.ts';
import type { PartsAnswer } from './parts.ts';

export const partsNow = $state<{ answer: PartsAnswer | null }>({ answer: null });

export async function refreshParts(): Promise<void> {
	try {
		partsNow.answer = await api.get<PartsAnswer>('/parts');
	} catch {
	}
}

const find = (name: string) => partsNow.answer?.parts.find((p) => p.part === name);

export const partOn = (name: string): boolean => !!partsNow.answer && find(name)?.on !== false;

export const partOff = (name: string): boolean => find(name)?.on === false;
