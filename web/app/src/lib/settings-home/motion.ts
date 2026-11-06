// Settings motion: one easing family (the app's --ease-out, a soft ease-out
// with no overshoot), transform and opacity only, and every animation can be
// cancelled or turned around mid-flight.
//
// Desktop and tablet: a card zooms into the page (a shared element
// transition: the card's surface grows to fill the content column while the
// other cards step back), and Back zooms the page into its card again.
// Phones: a native push (the page slides in from the right, Settings slides
// a little left under it) and the same move backwards on Back.

export const EASE = 'cubic-bezier(.22, 1, .36, 1)';
export const MS = {
	/** Card surface growing into the page, and shrinking back. */
	zoom: 460,
	zoomBack: 400,
	/** The other cards stepping back or forward. */
	recede: 280,
	/** The card's picture fading while it travels. */
	artFade: 200,
	/** Page coming up under the grown card, and the grown card clearing. */
	reveal: 240,
	/** Phone push and pop. */
	push: 380,
	pop: 320,
	/** A page inside the same group. */
	swap: 200
} as const;

export interface Box {
	left: number;
	top: number;
	width: number;
	height: number;
}

/**
 * flip is the transform that makes an element laid out at `to` look as if
 * it sits at `from` (scale from its top-left corner). Animate it to 'none'
 * to grow from `from` into `to`; from 'none' to it to shrink back.
 */
export function flip(from: Box, to: Box): string {
	const sx = to.width > 0 ? from.width / to.width : 1;
	const sy = to.height > 0 ? from.height / to.height : 1;
	const r = (n: number) => Math.round(n * 1000) / 1000;
	return `translate(${r(from.left - to.left)}px, ${r(from.top - to.top)}px) scale(${r(sx)}, ${r(sy)})`;
}

/** shift is a plain move (no scale), for pieces that must not stretch. */
export function shift(from: Box, to: Box): string {
	const r = (n: number) => Math.round(n * 1000) / 1000;
	return `translate(${r(from.left - to.left)}px, ${r(from.top - to.top)}px)`;
}

/** reduced says whether the person asked for less motion. */
export function reduced(): boolean {
	return typeof matchMedia === 'function' && matchMedia('(prefers-reduced-motion: reduce)').matches;
}

/** phone is the push width; wider screens zoom. */
export function phone(): boolean {
	return typeof innerWidth === 'number' && innerWidth < 768;
}

/** Direction of the last move in Settings, read by the page that mounts next. */
export type Move = 'open' | 'back' | 'swap' | null;
