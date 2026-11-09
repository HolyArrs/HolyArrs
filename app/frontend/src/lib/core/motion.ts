// Motion helpers: press and reveal actions, view transitions and card zoom maths.

import type { Action } from 'svelte/action';

type MediaWindow = { matchMedia?: (q: string) => { matches: boolean } };

export type PressKind = 'button' | 'round' | 'card' | 'row';

type TransitionDoc = { startViewTransition?: (cb: () => Promise<void> | void) => unknown };

type PageChange = { complete: Promise<void>; from?: { url: URL } | null; to?: { url: URL } | null };

export interface Box {
	left: number;
	top: number;
	width: number;
	height: number;
}

export type Move = 'open' | 'back' | 'swap' | null;

// Any of these ends a press, so the pressed look never sticks.
const releaseEvents = ['pointerup', 'pointerleave', 'pointercancel', 'keyup', 'blur'] as const;

// Theater open and close timings in ms, and the shared ease curve.
export const THEATER_GROW_MS = 420;
export const THEATER_SHRINK_MS = 380;
export const THEATER_CHROME_MS = 150;
export const THEATER_FADE_MS = 200;
export const THEATER_EASE = 'cubic-bezier(.22, 1, .36, 1)';

// Shared ease and durations in ms for sheet and page moves.
export const EASE = 'cubic-bezier(.22, 1, .36, 1)';
export const MS = {
	zoom: 420,
	zoomBack: 380,
	cover: 120,
	recede: 280,
	artFade: 200,
	reveal: 220,
	push: 380,
	pop: 320,
	swap: 200
} as const;

// Reads the reduced-motion setting, safe where no window exists.
export function reducedMotion(win: MediaWindow | undefined = typeof window === 'undefined' ? undefined : window): boolean {
	try {
		return !!win?.matchMedia?.('(prefers-reduced-motion: reduce)').matches;
	} catch {
		return false;
	}
}

// Scrolls an element to the top of the view, jumping instead of gliding when motion is reduced.
export function scrollToStart(el: Element | null | undefined): void {
	el?.scrollIntoView({ behavior: reducedMotion() ? 'auto' : 'smooth', block: 'start' });
}

// Caps the stagger so long lists do not trail for seconds.
export function staggerDelay(index: number, step = 30, max = 8, reduce = false): number {
	if (reduce || !Number.isFinite(index) || index <= 0) return 0;
	return Math.min(Math.floor(index), max - 1) * step;
}

export function pressScale(kind: PressKind, reduce = false): number {
	if (reduce) return 1;
	return { button: 0.97, round: 0.92, card: 0.98, row: 0.98 }[kind];
}

// Sets a pressed flag the styles use to scale the element.
export const press: Action<HTMLElement, PressKind | undefined> = (node, kind) => {
	let current: PressKind = kind ?? 'button';
	node.dataset.press = '';
	const apply = () => node.style.setProperty('--press-scale', String(pressScale(current)));
	apply();
	const blocked = () => node.matches(':disabled, [aria-disabled="true"]');
	const down = (e: PointerEvent) => {
		if (e.button !== 0 || blocked()) return;
		node.dataset.pressed = '';
	};
	const key = (e: KeyboardEvent) => {
		if ((e.key === ' ' || e.key === 'Enter') && !e.repeat && !blocked()) node.dataset.pressed = '';
	};
	const up = () => {
		delete node.dataset.pressed;
	};
	node.addEventListener('pointerdown', down);
	node.addEventListener('keydown', key);
	for (const ev of releaseEvents) node.addEventListener(ev, up);
	return {
		update(next) {
			current = next ?? 'button';
			apply();
		},
		destroy() {
			node.removeEventListener('pointerdown', down);
			node.removeEventListener('keydown', key);
			for (const ev of releaseEvents) node.removeEventListener(ev, up);
			delete node.dataset.press;
			delete node.dataset.pressed;
		}
	};
};

// Fades items in when they scroll into view; the timer clears the helper styles after the transition.
export const reveal: Action<HTMLElement, { index?: number } | number | undefined> = (node, param) => {
	const index = typeof param === 'number' ? param : (param?.index ?? 0);
	if (reducedMotion() || typeof IntersectionObserver === 'undefined') return {};
	node.dataset.reveal = 'wait';
	let timer: ReturnType<typeof setTimeout> | undefined;
	const io = new IntersectionObserver(
		(entries) => {
			if (!entries.some((e) => e.isIntersecting)) return;
			const delay = staggerDelay(index);
			node.style.transitionDelay = `${delay}ms`;
			node.dataset.reveal = 'in';
			io.disconnect();
			timer = setTimeout(() => {
				node.style.transitionDelay = '';
				delete node.dataset.reveal;
			}, delay + 420);
		},
		{ rootMargin: '0px 0px -4% 0px' }
	);
	io.observe(node);
	return {
		destroy() {
			io.disconnect();
			if (timer) clearTimeout(timer);
		}
	};
};

// View transition names cannot start with a digit or hold odd characters.
export function safeTransitionName(name: string): string {
	const clean = name.replace(/[^a-zA-Z0-9_-]/g, '-');
	return /^[a-zA-Z_]/.test(clean) ? clean : `vt-${clean}`;
}

export const viewTransitionName: Action<HTMLElement, string | undefined | null> = (node, name) => {
	const set = (n: string | undefined | null) => {
		if (n) node.style.setProperty('view-transition-name', safeTransitionName(n));
		else node.style.removeProperty('view-transition-name');
	};
	set(name);
	return { update: set, destroy: () => set(null) };
};

export function canViewTransition(doc: TransitionDoc | undefined = defaultDoc(), reduce = reducedMotion()): boolean {
	return !reduce && typeof doc?.startViewTransition === 'function';
}

// Skips same-page and settings tab moves, where a page-wide transition would feel wrong.
export function navigateWithTransition(navigation: PageChange, doc: TransitionDoc | undefined = defaultDoc()): Promise<void> | undefined {
	if (!canViewTransition(doc)) return;
	if (navigation.from && navigation.to && navigation.from.url.pathname === navigation.to.url.pathname) return;
	const inSettings = (u?: { url: URL } | null) => !!u && /\/settings(?:\/|$)/.test(u.url.pathname);
	if (inSettings(navigation.from) && inSettings(navigation.to)) return;
	const start = doc!.startViewTransition!;
	return new Promise((resolve) => {
		start.call(doc, async () => {
			resolve();
			await navigation.complete;
		});
	});
}

// Largest box of the given aspect that fits the screen; bad ratios fall back to 16:9.
export function containBox(aspect: number, vw: number, vh: number): Box {
	const a = aspect > 0 && Number.isFinite(aspect) ? aspect : 16 / 9;
	const width = Math.min(vw, vh * a);
	const height = width / a;
	return { left: (vw - width) / 2, top: (vh - height) / 2, width, height };
}

// Solves the curve by bisection so frames can match the CSS ease.
export function cubicBezier(x1: number, y1: number, x2: number, y2: number): (t: number) => number {
	const at = (a: number, b: number, s: number) => 3 * a * s * (1 - s) ** 2 + 3 * b * s * s * (1 - s) + s ** 3;
	return (t) => {
		if (t <= 0) return 0;
		if (t >= 1) return 1;
		let lo = 0;
		let hi = 1;
		let s = t;
		for (let i = 0; i < 30; i++) {
			s = (lo + hi) / 2;
			if (at(x1, x2, s) < t) lo = s;
			else hi = s;
		}
		return at(y1, y2, s);
	};
}
export const theaterEase = cubicBezier(0.22, 1, 0.36, 1);

// Frames for growing a card to full screen; outer scales the frame, inner cancels the stretch so the picture keeps its shape.
export function flipFrames(from: Box, aspect: number, vw: number, vh: number, shrink = false, steps = 24, ease = theaterEase): { outer: string[]; inner: string[] } {
	const c = containBox(aspect, vw, vh);
	const s0 = Math.max(from.width / c.width, from.height / c.height);
	const ccx = c.left + c.width / 2;
	const ccy = c.top + c.height / 2;
	const outer: string[] = [];
	const inner: string[] = [];
	for (let i = 0; i <= steps; i++) {
		const p = shrink ? 1 - ease(i / steps) : ease(i / steps);
		const w = from.width + (vw - from.width) * p;
		const h = from.height + (vh - from.height) * p;
		const tx = from.left * (1 - p);
		const ty = from.top * (1 - p);
		const ox = w / vw;
		const oy = h / vh;
		const s = s0 + (1 - s0) * p;
		const px = tx + w / 2 - s * ccx;
		const py = ty + h / 2 - s * ccy;
		outer.push(`translate(${round(tx)}px, ${round(ty)}px) scale(${round(ox, 5)}, ${round(oy, 5)})`);
		inner.push(`translate(${round((px - tx) / ox)}px, ${round((py - ty) / oy)}px) scale(${round(s / ox, 5)}, ${round(s / oy, 5)})`);
	}
	return { outer, inner };
}

// Transform that makes the target box look like the source box.
export function flip(from: Box, to: Box): string {
	const sx = to.width > 0 ? from.width / to.width : 1;
	const sy = to.height > 0 ? from.height / to.height : 1;
	const r = (n: number) => Math.round(n * 1000) / 1000;
	return `translate(${r(from.left - to.left)}px, ${r(from.top - to.top)}px) scale(${r(sx)}, ${r(sy)})`;
}

export function shift(from: Box, to: Box): string {
	const r = (n: number) => Math.round(n * 1000) / 1000;
	return `translate(${r(from.left - to.left)}px, ${r(from.top - to.top)}px)`;
}

export function reduced(): boolean {
	return typeof matchMedia === 'function' && matchMedia('(prefers-reduced-motion: reduce)').matches;
}

// Under 768 px counts as a phone.
export function phone(): boolean {
	return typeof innerWidth === 'number' && innerWidth < 768;
}

// Keeps the corner radius looking the same while the sheet scales from card size.
export function radiusFor(card: Box, sheet: Box, cardRadius = 16): number {
	const s = sheet.width > 0 ? Math.min(1, card.width / sheet.width) : 1;
	return Math.round(cardRadius / Math.sqrt(Math.max(s, 0.05)));
}

function defaultDoc(): TransitionDoc | undefined {
	return typeof document === 'undefined' ? undefined : (document as unknown as TransitionDoc);
}

// Rounding keeps generated transform strings short.
function round(n: number, places = 2): number {
	const f = 10 ** places;
	return Math.round(n * f) / f;
}
