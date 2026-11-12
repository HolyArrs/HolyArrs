// Where a floating panel goes next to its button: the roomier side, 8px inside the viewport,
// start and end following the reading direction, and a height cap when neither side fits.
export type Rect = { left: number; top: number; right: number; bottom: number };
export type Placed = { left: number; top?: number; bottom?: number; up: boolean; maxHeight?: number; originX: 'left' | 'right' };

export function placePanel(
	b: Rect,
	width: number,
	h: number,
	view: { width: number; height: number },
	{ align = 'start', placement = 'below', rtl = false }: { align?: 'start' | 'end'; placement?: 'below' | 'above'; rtl?: boolean }
): Placed {
	const vw = view.width;
	const vh = view.height;
	const w = Math.min(width, vw - 24);
	const below = vh - b.bottom - 6;
	const above = b.top - 6;
	const up = placement === 'above' ? above >= h || b.top > vh - b.bottom : below < h && b.top > vh - b.bottom;
	const leftEdge = (align === 'start') !== rtl;
	let left = leftEdge ? b.left : b.right - w;
	left = Math.max(8, Math.min(left, vw - 8 - w));
	const room = up ? above : below;
	const maxHeight = h > room ? Math.max(0, room - 8) : undefined;
	const originX = leftEdge ? 'left' : 'right';
	return up ? { left, bottom: vh - b.top + 6, up, maxHeight, originX } : { left, top: b.bottom + 6, up, maxHeight, originX };
}
