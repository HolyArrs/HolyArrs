// Timing rules that decide when a loading page shows skeleton, orbit and reassurance.
import { t } from '#lib/core/i18n/i18n.svelte.ts';
export type LoadStage = 'quiet' | 'skeleton' | 'orbit' | 'reassure';

// Wait a beat before showing anything, so quick loads never flash.
export const SHOW_AFTER_MS = 150;
export const MIN_SHOW_MS = 100;
export const FADE_MS = 200;
export const ORBIT_AFTER_MS = 500;
export const REASSURE_AFTER_MS = 10_000;

export function loadStage(elapsed: number): LoadStage {
	if (elapsed >= REASSURE_AFTER_MS) return 'reassure';
	if (elapsed >= ORBIT_AFTER_MS) return 'orbit';
	if (elapsed >= SHOW_AFTER_MS) return 'skeleton';
	return 'quiet';
}

// Once shown, keep it up a minimum time so it does not blink away.
export function holdFor(elapsed: number, cached = false): number {
	if (cached || elapsed < SHOW_AFTER_MS) return 0;
	return Math.max(0, SHOW_AFTER_MS + MIN_SHOW_MS - elapsed);
}

// Timers still to fire for stages the load has not reached yet.
export function stageTimers(elapsed: number): { at: number; stage: LoadStage }[] {
	return (
		[
			{ at: SHOW_AFTER_MS, stage: 'skeleton' },
			{ at: ORBIT_AFTER_MS, stage: 'orbit' },
			{ at: REASSURE_AFTER_MS, stage: 'reassure' }
		] as const
	)
		.filter((t) => t.at > elapsed)
		.map((t) => ({ at: t.at - elapsed, stage: t.stage }));
}

export function buildingStatus(attempt: number, what = t('core.silhouette.thisPage')): string {
	if (attempt <= 1) return t('core.silhouette.building', { what });
	return t('core.silhouette.buildingTry', { what, attempt });
}

