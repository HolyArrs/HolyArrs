// The motion tokens (tokens.css) for code: Svelte transitions and element.animate()
// use these so a popover, a toast and a moving card keep the same pace as CSS.
// Under reduced motion every helper turns into a short crossfade, nothing travels.

import { fade, fly, scale, type TransitionConfig } from 'svelte/transition';
import { flip } from 'svelte/animate';
import { reducedMotion } from '../motion.ts';

/** Durations in ms, the same as tokens.css. */
export const DUR = { instant: 90, fast: 150, base: 240, slow: 360, page: 300, glide: 420, hero: 900 } as const;

/** Eases as CSS strings (element.animate), the same as tokens.css. */
export const EASE = {
	out: 'cubic-bezier(.22, 1, .36, 1)',
	in: 'cubic-bezier(.4, 0, 1, 1)',
	inOut: 'cubic-bezier(.65, 0, .35, 1)',
	glide: 'cubic-bezier(.32, .72, 0, 1)'
} as const;

/** Eases as functions (Svelte transitions). */
export const easeOut = (t: number) => 1 - Math.pow(1 - t, 4);
export const easeIn = (t: number) => t * t * t;
/** A soft overshoot for pop-ins, about 6% past the end, the same feel as --ease-spring. */
export const easeSpring = (t: number) => {
	const c = 1.2;
	return 1 + (c + 1) * Math.pow(t - 1, 3) + c * Math.pow(t - 1, 2);
};

/** The fade reduced motion gets in place of any travel. */
function calm(node: Element, duration: number): TransitionConfig {
	return fade(node, { duration });
}

/** A floating panel (popover, menu) entering: grows a little from its anchor. */
export function popIn(node: Element): TransitionConfig {
	if (reducedMotion()) return calm(node, DUR.fast);
	return scale(node, { start: 0.96, opacity: 0, duration: DUR.base, easing: easeSpring });
}
/** A floating panel leaving: quicker than it came, accelerating away. */
export function popOut(node: Element): TransitionConfig {
	if (reducedMotion()) return calm(node, DUR.fast);
	return scale(node, { start: 0.97, opacity: 0, duration: DUR.fast, easing: easeIn });
}

/** A notification sliding up from the bottom. */
export function dropIn(node: Element): TransitionConfig {
	if (reducedMotion()) return calm(node, DUR.fast);
	return fly(node, { y: 24, duration: DUR.slow, easing: easeSpring });
}
/** A notification leaving downwards. */
export function dropOut(node: Element): TransitionConfig {
	if (reducedMotion()) return calm(node, DUR.fast);
	return fly(node, { y: 16, duration: DUR.fast, easing: easeIn });
}

/** How long a list item takes to slide to its new place (animate:flip). 0 under reduced motion. */
export function moveMs(): number {
	return reducedMotion() ? 0 : DUR.base;
}

/** The scroll behaviour for a jump within a page: smooth, or instant under reduced motion. */
export function glideScroll(): ScrollBehavior {
	return reducedMotion() ? 'auto' : 'smooth';
}

/**
 * glide slides the items of a keyed list into their new places when one is
 * added, removed or moved (animate:glide), so the rest never jump. Instant
 * under reduced motion.
 */
export function glide(node: Element, rects: { from: DOMRect; to: DOMRect }) {
	return flip(node, rects, { duration: moveMs(), easing: easeOut });
}
