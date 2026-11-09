// Geometry and timing for the card that grows over its art on hover.


export interface Rect {
	left: number;
	top: number;
	width: number;
	height: number;
}

export interface Growth {
	left: number;
	top: number;
	width: number;
	mediaHeight: number;
	from: { x: number; y: number; scale: number };
}

// Delay and trailer start are in ms; the rest are sizes in px.
export const GROW_SCALE = 1.5;
export const GROW_WIDTH = 414;
export const GROW_MIN = 240;
export const GROW_MAX = 560;
export const GROW_MS = 220;
export const SHRINK_MS = 180;
export const GROW_DELAY = 400;
export const TRAILER_AFTER = 0;

// Keeps the grown card inside the view and bounds, and returns the offset to animate from.
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
	// 16:9 when the art has no size yet.
	const ratio = art.width > 0 ? art.height / art.width : 9 / 16;
	const mediaHeight = Math.round(width * ratio);
	const total = mediaHeight + panelHeight;

	const cx = art.left + art.width / 2;
	let left = cx - width / 2;
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

// Runs at most max jobs at once, in arrival order.
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

// Starting transform so the card appears to grow out of the art.
export function flipTransform(f: Growth['from']): string {
	return `translate(${f.x.toFixed(2)}px, ${f.y.toFixed(2)}px) scale(${f.scale.toFixed(4)})`;
}
