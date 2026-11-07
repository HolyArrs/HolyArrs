// Rows render only what is near the screen: a row far below waits as a
// fixed-height placeholder, and a row's cards come in a dozen at a time as it
// scrolls sideways. Rows also keep their places once shown, so a row that
// answers late never moves the rows around it.

/** Cards a row renders at first. */
export const ROW_FIRST = 12;
/** Cards added each time a row nears its last rendered card. */
export const ROW_STEP = 12;
/** A row mounts once it is within this many screens below the viewport. */
export const MOUNT_SCREENS = 1.5;
/** Rows the page renders at once, without waiting to come near the screen. */
export const EAGER_ROWS = 3;

/** mountNow says whether a row whose top is at `top` (px from the viewport top) is near enough to render. */
export function mountNow(top: number, viewport: number, screens = MOUNT_SCREENS): boolean {
	return top < viewport * (1 + screens);
}

/** nextLimit grows a row's rendered cards once less than half a width is left past what shows. */
export function nextLimit(limit: number, total: number, s: { scrollLeft: number; clientWidth: number; scrollWidth: number }): number {
	if (limit >= total) return limit;
	if (s.scrollLeft + s.clientWidth * 1.5 < s.scrollWidth) return limit;
	return Math.min(total, limit + ROW_STEP);
}

/**
 * stableRows keeps the order rows were first shown in during this visit:
 * known rows keep their place (with their new cards), rows that are gone
 * drop out, and new rows go after the known ones in the server's order.
 * It answers the rows and the order to keep for next time.
 */
export function stableRows<R>(order: readonly string[], rows: readonly R[], id: (r: R) => string): { rows: R[]; order: string[] } {
	const byId = new Map(rows.map((r) => [id(r), r]));
	const known = new Set(order);
	const kept = order.filter((k) => byId.has(k)).map((k) => byId.get(k)!);
	const fresh = rows.filter((r) => !known.has(id(r)));
	const out = [...kept, ...fresh];
	return { rows: out, order: out.map(id) };
}
