
export const ROW_FIRST = 12;
export const ROW_STEP = 12;
export const MOUNT_SCREENS = 1.5;
export const EAGER_ROWS = 3;

export function mountNow(top: number, viewport: number, screens = MOUNT_SCREENS): boolean {
	return top < viewport * (1 + screens);
}

export function nextLimit(limit: number, total: number, s: { scrollLeft: number; clientWidth: number; scrollWidth: number }): number {
	if (limit >= total) return limit;
	if (s.scrollLeft + s.clientWidth * 1.5 < s.scrollWidth) return limit;
	return Math.min(total, limit + ROW_STEP);
}

export function stableRows<R>(order: readonly string[], rows: readonly R[], id: (r: R) => string): { rows: R[]; order: string[] } {
	const byId = new Map(rows.map((r) => [id(r), r]));
	const known = new Set(order);
	const kept = order.filter((k) => byId.has(k)).map((k) => byId.get(k)!);
	const fresh = rows.filter((r) => !known.has(id(r)));
	const out = [...kept, ...fresh];
	return { rows: out, order: out.map(id) };
}
