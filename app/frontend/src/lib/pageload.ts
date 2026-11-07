// Page loading rules (Orbit loading screen, 2026-10-21). Pure functions so they can be
// tested; lib/ui/PageLoader.svelte applies them to the content area of every page.

/** The Orbit shows only when a page takes longer than this; faster pages just appear. */
export const PAGE_SHOW_AFTER_MS = 300;
/** Once shown, the Orbit stays at least this long, so it never blinks (speed waves 2026-10-27: was 500). */
export const PAGE_MIN_SHOW_MS = 100;
/** Content fades in over this long. */
export const PAGE_FADE_MS = 200;
/** A status line joins the Orbit after this. */
export const PAGE_STATUS_AFTER_MS = 3_000;
/** Reassurance and a retry after this. */
export const PAGE_REASSURE_AFTER_MS = 10_000;

export type PageStage = 'hidden' | 'orbit' | 'status' | 'reassure';

/** What a page still loading shows, `elapsed` ms after it started. */
export function pageStage(elapsed: number): PageStage {
	if (elapsed >= PAGE_REASSURE_AFTER_MS) return 'reassure';
	if (elapsed >= PAGE_STATUS_AFTER_MS) return 'status';
	if (elapsed >= PAGE_SHOW_AFTER_MS) return 'orbit';
	return 'hidden';
}

/** How long to keep the Orbit after the data arrived `elapsed` ms after the start;
 * nothing when the answer came from the app's kept answers (`cached`). */
export function pageHoldFor(elapsed: number, cached = false): number {
	if (cached || elapsed < PAGE_SHOW_AFTER_MS) return 0;
	return Math.max(0, PAGE_SHOW_AFTER_MS + PAGE_MIN_SHOW_MS - elapsed);
}

/** Timers a loading page needs: when each later stage begins, counted from `elapsed`. */
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

/**
 * The line under the Orbit: nothing early, the page's own text (or a default) after 3 s.
 * `now` shows the page's text as soon as the Orbit does (a "building" answer and its try count).
 */
export function pageLine(stage: PageStage, status = '', now = false): string {
	if (stage === 'hidden') return '';
	if (stage === 'orbit') return now ? status : '';
	return status || 'Getting things ready…';
}
