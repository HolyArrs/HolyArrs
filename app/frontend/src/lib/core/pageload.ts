// Timing rules for the page loading indicator.

import { t } from '#lib/core/i18n/i18n.svelte.ts';
export type PageStage = 'hidden' | 'orbit' | 'status' | 'reassure';

// Fast loads never show the loader.
export const PAGE_SHOW_AFTER_MS = 300;
export const PAGE_MIN_SHOW_MS = 100;
export const PAGE_FADE_MS = 200;
export const PAGE_STATUS_AFTER_MS = 3_000;
export const PAGE_REASSURE_AFTER_MS = 10_000;

// Keeps a shown loader up for a minimum time so it does not flash.
export function pageHoldFor(elapsed: number, cached = false): number {
	if (cached || elapsed < PAGE_SHOW_AFTER_MS) return 0;
	return Math.max(0, PAGE_SHOW_AFTER_MS + PAGE_MIN_SHOW_MS - elapsed);
}

// Stage changes still to come, as delays from now.
export function pageTimers(elapsed: number): { at: number; stage: PageStage }[] {
	return (
		[
			{ at: PAGE_SHOW_AFTER_MS, stage: 'orbit' },
			{ at: PAGE_STATUS_AFTER_MS, stage: 'status' },
			{ at: PAGE_REASSURE_AFTER_MS, stage: 'reassure' }
		] as const
	)
		.filter((t) => t.at > elapsed)
		.map((t) => ({ at: t.at - elapsed, stage: t.stage }));
}

// Text under the loader; the orbit stage stays quiet until asked.
export function pageLine(stage: PageStage, status = '', now = false): string {
	if (stage === 'hidden') return '';
	if (stage === 'orbit') return now ? status : '';
	return status || t('core.pageload.gettingReady');
}
