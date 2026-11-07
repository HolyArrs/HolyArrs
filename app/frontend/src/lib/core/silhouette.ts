// Silhouette timing rules (loading audit 2026-10-21). Pure functions so they can be tested;
// lib/ui/sk/Silhouette.svelte and the boot screen apply them.

/** The skeleton becomes visible this long after loading starts (it holds its space from frame one). */
export const SHOW_AFTER_MS = 150;
/** Once visible, the skeleton stays at least this long, so it never flickers (speed waves 2026-10-27: was 400). */
export const MIN_SHOW_MS = 100;
/** Content cross-fades in over this long. */
export const FADE_MS = 200;
/** After this, the in-region Orbit and a status line join the skeleton. */
export const ORBIT_AFTER_MS = 500;
/** After this, a reassurance line and a retry. */
export const REASSURE_AFTER_MS = 10_000;
/** Boot screen: a status line after this… */
export const BOOT_STATUS_MS = 3_000;
/** …and a reload option after this. */
export const BOOT_RELOAD_MS = 10_000;

export type LoadStage = 'quiet' | 'skeleton' | 'orbit' | 'reassure';

/** What a region still loading shows, `elapsed` ms after it started. */
export function loadStage(elapsed: number): LoadStage {
	if (elapsed >= REASSURE_AFTER_MS) return 'reassure';
	if (elapsed >= ORBIT_AFTER_MS) return 'orbit';
	if (elapsed >= SHOW_AFTER_MS) return 'skeleton';
	return 'quiet';
}

/**
 * How long to keep the skeleton after the data arrived `elapsed` ms after the start.
 * Data before the skeleton showed, or answered from the app's kept answers (`cached`),
 * swaps in at once; otherwise the skeleton stays until it has been visible for MIN_SHOW_MS.
 */
export function holdFor(elapsed: number, cached = false): number {
	if (cached || elapsed < SHOW_AFTER_MS) return 0;
	return Math.max(0, SHOW_AFTER_MS + MIN_SHOW_MS - elapsed);
}

/** Timers a loading region needs: when each later stage begins, from now. */
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

/** The status under a "building" answer: says it is working and counts the tries. */
export function buildingStatus(attempt: number, what = 'this page'): string {
	if (attempt <= 1) return `Building ${what}…`;
	return `Building ${what}… try ${attempt}`;
}

/** The boot screen's line `elapsed` ms after the page started. */
export function bootLine(elapsed: number): { text: string; reload: boolean } {
	if (elapsed >= BOOT_RELOAD_MS) return { text: 'This is taking longer than usual. The server may still be starting.', reload: true };
	if (elapsed >= BOOT_STATUS_MS) return { text: 'Starting HolyArrs…', reload: false };
	return { text: '', reload: false };
}
