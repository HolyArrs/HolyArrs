// An element's width, as one shared ResizeObserver reports it. Unlike bind:clientWidth it never asks the browser to lay the page out again on the spot, which long lists of cards did once per card as they scrolled in.

const report = new WeakMap<Element, (width: number) => void>();
let observer: ResizeObserver | undefined;

function seen(entries: ResizeObserverEntry[]) {
	for (const e of entries) report.get(e.target)?.(Math.round(e.contentRect.width));
}

/** A Svelte action that calls `set` with the element's content width (whole pixels) when it is first laid out and whenever it changes. */
export function boxWidth(node: Element, set: (width: number) => void) {
	report.set(node, set);
	if (typeof ResizeObserver !== 'undefined') (observer ??= new ResizeObserver(seen)).observe(node);
	return {
		update(next: (width: number) => void) {
			report.set(node, next);
		},
		destroy() {
			observer?.unobserve(node);
			report.delete(node);
		}
	};
}
