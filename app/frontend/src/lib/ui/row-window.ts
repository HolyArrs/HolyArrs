// Limits for how many rows and cards are mounted so long pages stay light.

export const ROW_FIRST = 12;
export const ROW_STEP = 12;
// Rows mount when within this many screens below the fold.
export const MOUNT_SCREENS = 1.5;
export const EAGER_ROWS = 3;

// True when a row is near enough to the viewport to render.
export function mountNow(top: number, viewport: number, screens = MOUNT_SCREENS): boolean {
	return top < viewport * (1 + screens);
}

// Grows a row only when the user scrolls within 1.5 widths of its end.
export function nextLimit(limit: number, total: number, s: { scrollLeft: number; clientWidth: number; scrollWidth: number }): number {
	if (limit >= total) return limit;
	if (s.scrollLeft + s.clientWidth * 1.5 < s.scrollWidth) return limit;
	return Math.min(total, limit + ROW_STEP);
}

// Keeps known rows in place and appends new ones, so rows do not jump when data refreshes.
export function stableRows<R>(order: readonly string[], rows: readonly R[], id: (r: R) => string): { rows: R[]; order: string[] } {
	const byId = new Map(rows.map((r) => [id(r), r]));
	const known = new Set(order);
	const kept = order.filter((k) => byId.has(k)).map((k) => byId.get(k)!);
	const fresh = rows.filter((r) => !known.has(id(r)));
	const out = [...kept, ...fresh];
	return { rows: out, order: out.map(id) };
}
