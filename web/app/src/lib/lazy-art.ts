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

/** nearView calls near once, the first time node comes within nearMargin of the screen. */
export function nearView(node: Element, near: () => void) {
	if (typeof IntersectionObserver === 'undefined') {
		near();
		return {};
	}
	const io = new IntersectionObserver(
		(es) => {
			if (es.some((e) => e.isIntersecting)) {
				io.disconnect();
				near();
			}
		},
		{ rootMargin: nearMargin }
	);
	io.observe(node);
	return { destroy: () => io.disconnect() };
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
