// Back from a title page lands on Explore where it was: the page's scroll
// and each row's sideways offset are kept in the history entry (SvelteKit
// snapshots) and put back once the rows have loaded far enough.

export interface ExploreReturn {
	/** The page's vertical scroll. */
	y: number;
	/** Each scrolled row's sideways offset, by row id. */
	rows: Record<string, number>;
}

/** readOffsets reads the sideways offset of every scrolled row under root. */
export function readOffsets(root: ParentNode): Record<string, number> {
	const out: Record<string, number> = {};
	for (const el of root.querySelectorAll<HTMLElement>('[data-row-id]')) {
		const row = el.querySelector<HTMLElement>('.ui-shelf .row, .row');
		const id = el.dataset.rowId;
		if (id && row && row.scrollLeft > 0) out[id] = Math.round(row.scrollLeft);
	}
	return out;
}

/** putOffsets puts the rows back; rows not on the page (yet) are skipped. */
export function putOffsets(root: ParentNode, rows: Record<string, number>): void {
	for (const el of root.querySelectorAll<HTMLElement>('[data-row-id]')) {
		const left = rows[el.dataset.rowId ?? ''];
		const row = el.querySelector<HTMLElement>('.ui-shelf .row, .row');
		if (left && row) row.scrollLeft = left;
	}
}

/** canReturn: the page is tall enough to scroll back to y (or nothing more will load). */
export function canReturn(pageHeight: number, y: number, viewport: number, more: boolean): boolean {
	return pageHeight >= y + viewport || !more;
}
