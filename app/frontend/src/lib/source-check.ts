import type { SourceHealth } from './sources/index.ts';
import type { Tone } from './core/format.ts';
import { sourceTone } from './sources.ts';

/** One Source's answer to a test search (POST /sources/check). */
export interface SourceCheck {
	id: number;
	name: string;
	answered: boolean;
	paused: boolean;
	count: number;
	ms: number;
	error?: string;
	health: SourceHealth;
}

export interface SourceCheckResult {
	query: string;
	kind: CheckKind;
	sources: SourceCheck[];
}

export type CheckKind = 'movie' | 'show' | 'music';

/** The title a test search asks for unless the owner types another. */
export const defaultQuery: Record<CheckKind, string> = { movie: 'The Matrix', show: 'Breaking Bad', music: 'Daft Punk' };

/** checkWord is the row's one word. */
export function checkWord(c: SourceCheck): string {
	if (c.paused) return 'Paused';
	return c.answered ? 'Connected' : 'Not connected';
}

/** checkTone colours the row with the same tones as a Source's health chip. */
export function checkTone(c: SourceCheck): Tone {
	if (c.paused) return sourceTone(c.health?.state ?? 'unreachable');
	return sourceTone(c.answered ? 'working' : 'unreachable');
}

/** checkCount says how many results came back. */
export function checkCount(c: SourceCheck): string {
	if (!c.answered) return '';
	return c.count === 1 ? '1 result' : `${c.count} results`;
}

/** checkSummary says the whole run in one line. */
export function checkSummary(list: SourceCheck[]): string {
	if (list.length === 0) return 'No enabled Sources to ask.';
	const ok = list.filter((c) => c.answered).length;
	const paused = list.filter((c) => c.paused).length;
	const failed = list.length - ok - paused;
	const parts = [`${ok} of ${list.length} connected`];
	if (failed) parts.push(`${failed} failed`);
	if (paused) parts.push(`${paused} paused`);
	return parts.join(', ') + '.';
}
