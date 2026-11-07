
export const SHOW_AFTER_MS = 150;
export const MIN_SHOW_MS = 100;
export const FADE_MS = 200;
export const ORBIT_AFTER_MS = 500;
export const REASSURE_AFTER_MS = 10_000;
export const BOOT_STATUS_MS = 3_000;
export const BOOT_RELOAD_MS = 10_000;

export type LoadStage = 'quiet' | 'skeleton' | 'orbit' | 'reassure';

export function loadStage(elapsed: number): LoadStage {
	if (elapsed >= REASSURE_AFTER_MS) return 'reassure';
	if (elapsed >= ORBIT_AFTER_MS) return 'orbit';
	if (elapsed >= SHOW_AFTER_MS) return 'skeleton';
	return 'quiet';
}

export function holdFor(elapsed: number, cached = false): number {
	if (cached || elapsed < SHOW_AFTER_MS) return 0;
	return Math.max(0, SHOW_AFTER_MS + MIN_SHOW_MS - elapsed);
}

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

export function buildingStatus(attempt: number, what = 'this page'): string {
	if (attempt <= 1) return `Building ${what}…`;
	return `Building ${what}… try ${attempt}`;
}

export function bootLine(elapsed: number): { text: string; reload: boolean } {
	if (elapsed >= BOOT_RELOAD_MS) return { text: 'This is taking longer than usual. The server may still be starting.', reload: true };
	if (elapsed >= BOOT_STATUS_MS) return { text: 'Starting HolyArrs…', reload: false };
	return { text: '', reload: false };
}
