import { getContext, setContext } from 'svelte';


const key = Symbol('shelf-near');

export const nearMargin = '600px 0px';

export function setShelfNear(near: () => boolean) {
	setContext(key, near);
}

export function shelfNear(): () => boolean {
	return getContext<(() => boolean) | undefined>(key) ?? (() => true);
}

export const FALLBACK_MS = 4000;

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

export function whenLoaded(img: HTMLImageElement, done: (img?: HTMLImageElement) => void) {
	if (img.complete && img.naturalWidth > 0) {
		done(img);
		return {};
	}
	const on = () => done(img);
	img.addEventListener('load', on, { once: true });
	return { destroy: () => img.removeEventListener('load', on) };
}
