
// Shared motion timings, easings and enter/exit transitions, calmer under reduced motion.
import { fade, fly, scale, slide, type TransitionConfig } from 'svelte/transition';
import { flip } from 'svelte/animate';
import { reducedMotion } from './motion.ts';

export const DUR = { instant: 90, fast: 150, base: 240, slow: 360, page: 300, glide: 420, hero: 900 } as const;

export const EASE = {
	out: 'cubic-bezier(.22, 1, .36, 1)',
	in: 'cubic-bezier(.4, 0, 1, 1)',
	inOut: 'cubic-bezier(.65, 0, .35, 1)',
	glide: 'cubic-bezier(.32, .72, 0, 1)'
} as const;

export const easeOut = (t: number) => 1 - Math.pow(1 - t, 4);
export const easeIn = (t: number) => t * t * t;
export const easeSpring = (t: number) => {
	const c = 1.2;
	return 1 + (c + 1) * Math.pow(t - 1, 3) + c * Math.pow(t - 1, 2);
};

// Reduced motion swaps movement for a plain fade.
function calm(node: Element, duration: number): TransitionConfig {
	return fade(node, { duration });
}

export function popIn(node: Element): TransitionConfig {
	if (reducedMotion()) return calm(node, DUR.fast);
	return scale(node, { start: 0.96, opacity: 0, duration: DUR.base, easing: easeSpring });
}
export function popOut(node: Element): TransitionConfig {
	if (reducedMotion()) return calm(node, DUR.fast);
	return scale(node, { start: 0.97, opacity: 0, duration: DUR.fast, easing: easeIn });
}

export function dropIn(node: Element): TransitionConfig {
	if (reducedMotion()) return calm(node, DUR.fast);
	return fly(node, { y: 24, duration: DUR.slow, easing: easeSpring });
}
export function dropOut(node: Element): TransitionConfig {
	if (reducedMotion()) return calm(node, DUR.fast);
	return fly(node, { y: 16, duration: DUR.fast, easing: easeIn });
}

export function notifyIn(node: Element): TransitionConfig {
	if (reducedMotion()) return calm(node, DUR.fast);
	return fly(node, { y: -14, duration: DUR.slow, easing: easeSpring });
}
export function notifyOut(node: Element): TransitionConfig {
	if (reducedMotion()) return calm(node, DUR.fast);
	return fly(node, { y: -10, duration: DUR.fast, easing: easeIn });
}

// Sheets fill the phone screen, so they appear without sliding.
const phone = () => typeof matchMedia === 'function' && matchMedia('(max-width: 640px)').matches;
export function sheetIn(node: Element): TransitionConfig {
	if (phone()) return { duration: 0 };
	if (reducedMotion()) return calm(node, DUR.fast);
	return slide(node, { duration: DUR.slow, easing: easeOut });
}
export function sheetOut(node: Element): TransitionConfig {
	if (phone()) return { duration: 0 };
	if (reducedMotion()) return calm(node, DUR.fast);
	return slide(node, { duration: DUR.base, easing: easeIn });
}

export function moveMs(): number {
	return reducedMotion() ? 0 : DUR.base;
}

export function glideScroll(): ScrollBehavior {
	return reducedMotion() ? 'auto' : 'smooth';
}

export function glide(node: Element, rects: { from: DOMRect; to: DOMRect }) {
	return flip(node, rects, { duration: moveMs(), easing: easeOut });
}
