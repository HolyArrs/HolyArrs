import type { SourceHealth } from './types.ts';
import type { Tone } from '../core/format.ts';
import { healthWords, sourceTone } from './api.ts';
import { t, appLocale } from '#lib/core/i18n/i18n.svelte.ts';

export const reasonWords: Record<NonNullable<SourceHealth['reason']>, string> = {
	get slowed() { return t('sources.reason.slowed'); },
	get down() { return t('sources.reason.down'); },
	get blocked() { return t('sources.reason.blocked'); },
	get key() { return t('sources.reason.key'); }
};

const reasonTone: Record<NonNullable<SourceHealth['reason']>, Tone> = { slowed: 'wait', down: 'fail', blocked: 'fail', key: 'fail' };

export function healthWord(h: Pick<SourceHealth, 'state' | 'reason'>): string {
	return (h.reason && reasonWords[h.reason]) || healthWords[h.state] || h.state;
}

// A slowed Source only waits; other reasons count as failed.
export function healthTone(h: Pick<SourceHealth, 'state' | 'reason'>): Tone {
	return (h.reason && reasonTone[h.reason]) || sourceTone(h.state);
}

export function pausedUntil(h: Pick<SourceHealth, 'until'>): string {
	if (!h.until) return '';
	const when = new Date(h.until);
	return Number.isNaN(when.getTime()) ? '' : t('sources.pausedUntil', { time: when.toLocaleTimeString(appLocale(), { hour: '2-digit', minute: '2-digit' }) });
}
