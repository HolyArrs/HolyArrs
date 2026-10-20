// The wide card's hover grow: where the grown card sits and the transform
// that makes it start exactly on the card it grows from (a FLIP). The grown
// card is about 1.5x the card, centred on it; at a row's edge it grows
// inward so it never leaves the row; it always stays inside the window.

export interface Rect {
	left: number;
	top: number;
	width: number;
	height: number;
}

export interface Growth {
	/** The grown card's place and width (fixed position, px). */
	left: number;
	top: number;
	width: number;
	/** The 16:9 picture's height inside the grown card. */
	mediaHeight: number;
	/** The start transform (origin top left): the grown card drawn over the card. */
	from: { x: number; y: number; scale: number };
}

export const GROW_SCALE = 1.5;
export const GROW_MIN = 240;
export const GROW_MAX = 560;
export const GROW_MS = 220;
/** The shrink back onto the card. */
export const SHRINK_MS = 180;
export const GROW_DELAY = 400;
/** After the card opens, the trailer player is put in (it shows only once playing): 0, at once. */
export const TRAILER_AFTER = 0;

/**
 * growPlacement places a grown card of the art's 16:9 ratio plus a panel
 * of panelHeight below the picture. The picture stays centred on the card
 * (both ways) unless a row edge or the window pushes it inward.
 */
export function growPlacement(
	art: Rect,
	panelHeight: number,
	view: { width: number; height: number },
	bounds?: { left: number; right: number },
	opts: { scale?: number; min?: number; max?: number; margin?: number } = {}
): Growth {
	const margin = opts.margin ?? 8;
	const lo = Math.max(margin, bounds?.left ?? margin);
	const hi = Math.min(view.width - margin, bounds?.right ?? view.width - margin);
	const room = Math.max(0, hi - lo);
	let width = Math.min(opts.max ?? GROW_MAX, Math.max(opts.min ?? GROW_MIN, art.width * (opts.scale ?? GROW_SCALE)));
	width = Math.round(Math.max(art.width, Math.min(width, room || width)));
	const ratio = art.width > 0 ? art.height / art.width : 9 / 16;
	const mediaHeight = Math.round(width * ratio);
	const total = mediaHeight + panelHeight;

	const cx = art.left + art.width / 2;
	let left = cx - width / 2;
	// At a row's edge: line up with the card's outer edge and grow inward.
	if (left < lo) left = Math.max(art.left, lo);
	if (left + width > hi) left = Math.min(art.left + art.width, hi) - width;
	if (left < lo) left = lo;

	const cy = art.top + art.height / 2;
	let top = cy - mediaHeight / 2;
	const bottomRoom = view.height - margin;
	if (top + total > bottomRoom) top = bottomRoom - total;
	if (top < margin) top = margin;

	const scale = width > 0 ? art.width / width : 1;
	return {
		left: Math.round(left),
		top: Math.round(top),
		width: Math.round(width),
		mediaHeight: Math.round(mediaHeight),
		from: { x: art.left - Math.round(left), y: art.top - Math.round(top), scale }
	};
}

/**
 * queue runs at most `max` jobs at a time (title logos for wide cards are
 * asked one card at a time as rows come into view; this keeps it gentle).
 */
export function queue(max = 4) {
	let busy = 0;
	const waiting: (() => void)[] = [];
	const next = () => {
		if (busy >= max) return;
		const job = waiting.shift();
		if (job) job();
	};
	return function run<T>(job: () => Promise<T>): Promise<T> {
		return new Promise<T>((resolve, reject) => {
			waiting.push(() => {
				busy++;
				job()
					.then(resolve, reject)
					.finally(() => {
						busy--;
						next();
					});
			});
			next();
		});
	};
}

/** Title logos for wide cards: four lookups at a time across the page. */
export const logoQueue = queue(4);

/** The CSS transform for a FLIP start (origin top left). */
export function flipTransform(f: Growth['from']): string {
	return `translate(${f.x.toFixed(2)}px, ${f.y.toFixed(2)}px) scale(${f.scale.toFixed(4)})`;
}
