// Back from a title page lands on Explore where it was. The page's scroll and each row's sideways offset are kept in the history entry (SvelteKit snapshots) and put back once the rows have loaded far enough.

export interface ExploreReturn {
	/** The page's vertical scroll. */
	y: number;
	/** Each scrolled row's sideways offset, by row id. */
	rows: Record<string, number>;
	/** The first row on screen and how far from the top it sat. Rows above may load at other heights, so this, not y, is the spot. */
	anchor?: { id: string; top: number };
}

type Box = { top: number; bottom: number };
type RowEl = { dataset: { rowId?: string }; getBoundingClientRect(): Box; firstElementChild?: { getBoundingClientRect(): Box } | null };

/** Finds the first row whose bottom is on screen (below the top bar) and its top. */
export function readAnchor(root: ParentNode, below = 0): { id: string; top: number } | undefined {
	for (const el of root.querySelectorAll('[data-row-id]') as unknown as Iterable<RowEl>) {
		const r = box(el);
		const id = el.dataset.rowId;
		if (id && r.bottom > below) return { id, top: Math.round(r.top) };
	}
	return undefined;
}

/** How far to scroll so the anchor row sits where it was; null when that row is not on the page. */
export function anchorDelta(root: ParentNode, anchor: { id: string; top: number } | undefined): number | null {
	if (!anchor) return null;
	for (const el of root.querySelectorAll('[data-row-id]') as unknown as Iterable<RowEl>) {
		if (el.dataset.rowId === anchor.id) return Math.round(box(el).top - anchor.top);
	}
	return null;
}

/** Reads the sideways offset of every scrolled row under root. */
export function readOffsets(root: ParentNode): Record<string, number> {
	const out: Record<string, number> = {};
	for (const el of root.querySelectorAll<HTMLElement>('[data-row-id]')) {
		const row = el.querySelector<HTMLElement>('.ui-shelf .row, .row');
		const id = el.dataset.rowId;
		if (id && row && row.scrollLeft > 0) out[id] = Math.round(row.scrollLeft);
	}
	return out;
}

/** Puts the rows back; rows not on the page (yet) are skipped. */
export function putOffsets(root: ParentNode, rows: Record<string, number>): void {
	for (const el of root.querySelectorAll<HTMLElement>('[data-row-id]')) {
		const left = rows[el.dataset.rowId ?? ''];
		const row = el.querySelector<HTMLElement>('.ui-shelf .row, .row');
		if (left && row) row.scrollLeft = left;
	}
}

/** The page is tall enough to scroll back to y, or nothing more will load. */
export function canReturn(pageHeight: number, y: number, viewport: number, more: boolean): boolean {
	return pageHeight >= y + viewport || !more;
}

/** A row wrapper may be display: contents (no box of its own); its first child stands in. */
function box(el: RowEl): Box {
	const r = el.getBoundingClientRect();
	return r.bottom - r.top > 0 || !el.firstElementChild ? r : el.firstElementChild.getBoundingClientRect();
}
