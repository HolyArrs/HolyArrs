import { getContext, setContext } from 'svelte';

// Lazy artwork for rows: a Shelf watches itself with one IntersectionObserver
// and tells its cards when it comes near the screen; only then do the cards
// put in their pictures (each still loading="lazy" for sideways scrolling).
// Until a picture has loaded, the card shows the title's name on a tinted tile.

const key = Symbol('shelf-near');

/** Margin around the screen at which a row starts loading its pictures. */
export const nearMargin = '600px 0px';

/** setShelfNear gives a Shelf's cards a way to ask whether it is near the screen. */
export function setShelfNear(near: () => boolean) {
	setContext(key, near);
}

/** shelfNear says whether the card's row is near the screen (true outside a row). */
export function shelfNear(): () => boolean {
	return getContext<(() => boolean) | undefined>(key) ?? (() => true);
}

/** How long a row waits for its observer before it loads anyway (when the browser is idle). */
export const FALLBACK_MS = 4000;

/**
 * whenIdle calls fn once after ms, at the browser's next idle moment, so a
 * row whose IntersectionObserver never fires (a hidden tab, a missed
 * callback) still loads. The returned function cancels it.
 */
export function whenIdle(fn: () => void, ms = FALLBACK_MS): () => void {
	let idle = 0;
	const t = setTimeout(() => {
		if (typeof requestIdleCallback === 'function') idle = requestIdleCallback(fn, { timeout: ms });
		else fn();
	}, ms);
	return () => {
		clearTimeout(t);
		if (idle && typeof cancelIdleCallback === 'function') cancelIdleCallback(idle);
	};
}

/** nearView calls near once, the first time node comes within nearMargin of the screen (or after the idle fallback). */
export function nearView(node: Element, near: () => void) {
	if (typeof IntersectionObserver === 'undefined') {
		near();
		return {};
	}
	let done = false;
	const once = () => {
		if (done) return;
		done = true;
		io.disconnect();
		stop();
		near();
	};
	const io = new IntersectionObserver((es) => es.some((e) => e.isIntersecting) && once(), { rootMargin: nearMargin });
	io.observe(node);
	const stop = whenIdle(once);
	return { destroy: () => { done = true; io.disconnect(); stop(); } };
}

/** whenLoaded calls done once the picture has loaded, also when it came from the cache before the listener. */
export function whenLoaded(img: HTMLImageElement, done: (img?: HTMLImageElement) => void) {
	if (img.complete && img.naturalWidth > 0) {
		done(img);
		return {};
	}
	const on = () => done(img);
	img.addEventListener('load', on, { once: true });
	return { destroy: () => img.removeEventListener('load', on) };
}
