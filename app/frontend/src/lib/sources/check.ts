import type { SourceHealth } from './types.ts';
import type { Tone } from '../core/format.ts';
import { sourceTone } from './api.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';

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
	if (c.paused) return t('sources.check.paused');
	return c.answered ? t('sources.check.connected') : t('sources.check.notConnected');
}

export function checkTone(c: SourceCheck): Tone {
	if (c.paused) return sourceTone(c.health?.state ?? 'unreachable');
	return sourceTone(c.answered ? 'working' : 'unreachable');
}

export function checkCount(c: SourceCheck): string {
	if (!c.answered) return '';
	return c.count === 1 ? t('sources.check.oneResult') : t('sources.check.results', { count: c.count });
}

export function checkSummary(list: SourceCheck[]): string {
	if (list.length === 0) return t('sources.check.none');
	const ok = list.filter((c) => c.answered).length;
	const paused = list.filter((c) => c.paused).length;
	const failed = list.length - ok - paused;
	const parts = [t('sources.check.summaryConnected', { ok, total: list.length })];
	if (failed) parts.push(t('sources.check.summaryFailed', { count: failed }));
	if (paused) parts.push(t('sources.check.summaryPaused', { count: paused }));
	return parts.join(', ') + '.';
}
