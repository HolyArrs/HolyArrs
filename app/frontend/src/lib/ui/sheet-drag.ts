// Drag-to-close for bottom sheets: the sheet follows the finger down and shuts past
// 30% of its height or on a quick flick; otherwise it springs back.

/** Faster than this (px per ms, downwards) counts as a flick. */
const FLICK = 0.11;

/** Whether a drag of `dy` px over `ms` milliseconds on a sheet `height` px tall closes it. */
export function dragShuts(dy: number, height: number, ms: number): boolean {
	if (dy <= 0) return false;
	if (dy > height * 0.3) return true;
	return dy / Math.max(ms, 1) > FLICK;
}

/** How far the sheet moves for a finger `dy` px from where it started: all the way down, a little up. */
export function dragOffset(dy: number): number {
	return dy >= 0 ? dy : -Math.min(16, Math.sqrt(-dy) * 1.5);
}
