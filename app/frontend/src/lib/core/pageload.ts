
export const PAGE_SHOW_AFTER_MS = 300;
export const PAGE_MIN_SHOW_MS = 100;
export const PAGE_FADE_MS = 200;
export const PAGE_STATUS_AFTER_MS = 3_000;
export const PAGE_REASSURE_AFTER_MS = 10_000;

export type PageStage = 'hidden' | 'orbit' | 'status' | 'reassure';

export function pageStage(elapsed: number): PageStage {
	if (elapsed >= PAGE_REASSURE_AFTER_MS) return 'reassure';
	if (elapsed >= PAGE_STATUS_AFTER_MS) return 'status';
	if (elapsed >= PAGE_SHOW_AFTER_MS) return 'orbit';
	return 'hidden';
}

export function pageHoldFor(elapsed: number, cached = false): number {
	if (cached || elapsed < PAGE_SHOW_AFTER_MS) return 0;
	return Math.max(0, PAGE_SHOW_AFTER_MS + PAGE_MIN_SHOW_MS - elapsed);
}

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

export function pageLine(stage: PageStage, status = '', now = false): string {
	if (stage === 'hidden') return '';
	if (stage === 'orbit') return now ? status : '';
	return status || 'Getting things ready…';
}
