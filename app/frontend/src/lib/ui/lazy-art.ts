import { getContext, setContext } from 'svelte';

// Loads art only when its shelf is near the screen, with an idle fallback.

const key = Symbol('shelf-near');

// Start loading this far before the shelf scrolls in.
export const nearMargin = '600px 0px';

// ms before art loads anyway, so nothing stays blank.
export const FALLBACK_MS = 4000;

export function setShelfNear(near: () => boolean) {
	setContext(key, near);
}

// Defaults to always near when no shelf provides it.
export function shelfNear(): () => boolean {
	return getContext<(() => boolean) | undefined>(key) ?? (() => true);
}

// Waits ms, then runs on an idle moment; returns a cancel.
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

// Runs near once, on first sight or after the fallback.
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

// Cached images are already complete and fire no load event.
export function whenLoaded(img: HTMLImageElement, done: (img?: HTMLImageElement) => void) {
	if (img.complete && img.naturalWidth > 0) {
		done(img);
		return {};
	}
	const on = () => done(img);
	img.addEventListener('load', on, { once: true });
	return { destroy: () => img.removeEventListener('load', on) };
}
