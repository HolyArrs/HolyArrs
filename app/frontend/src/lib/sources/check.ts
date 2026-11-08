import type { SourceHealth } from './types.ts';
import type { Tone } from '../core/format.ts';
import { sourceTone } from './api.ts';

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

export type CheckKind = 'movie' | 'show' | 'music';

export const defaultQuery: Record<CheckKind, string> = { movie: 'The Matrix', show: 'Breaking Bad', music: 'Daft Punk' };

export function checkWord(c: SourceCheck): string {
	if (c.paused) return 'Paused';
	return c.answered ? 'Connected' : 'Not connected';
}

export function checkTone(c: SourceCheck): Tone {
	if (c.paused) return sourceTone(c.health?.state ?? 'unreachable');
	return sourceTone(c.answered ? 'working' : 'unreachable');
}

export function checkCount(c: SourceCheck): string {
	if (!c.answered) return '';
	return c.count === 1 ? '1 result' : `${c.count} results`;
}

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
