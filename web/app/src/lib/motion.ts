// Calm motion for the Apple direction (spec §3). Svelte actions plus a page
// change helper. Timings live in tokens.css; these only toggle state, so
// reduced motion is honoured by the tokens as well as checked here.

import type { Action } from 'svelte/action';

type MediaWindow = { matchMedia?: (q: string) => { matches: boolean } };

/** reducedMotion says whether the person asked for less motion. */
export function reducedMotion(win: MediaWindow | undefined = typeof window === 'undefined' ? undefined : window): boolean {
	try {
		return !!win?.matchMedia?.('(prefers-reduced-motion: reduce)').matches;
	} catch {
		return false;
	}
}

/** staggerDelay is the entry delay for the i-th item: 30ms steps, capped at 8 items. */
export function staggerDelay(index: number, step = 30, max = 8, reduce = false): number {
	if (reduce || !Number.isFinite(index) || index <= 0) return 0;
	return Math.min(Math.floor(index), max - 1) * step;
}

export type PressKind = 'button' | 'round' | 'card' | 'row';

/** pressScale is how far each kind of control shrinks while held. */
export function pressScale(kind: PressKind, reduce = false): number {
	if (reduce) return 1;
	return { button: 0.97, round: 0.92, card: 0.98, row: 0.98 }[kind];
}

const releaseEvents = ['pointerup', 'pointerleave', 'pointercancel', 'keyup', 'blur'] as const;

/**
 * press shrinks a control while it is held and lets it spring back on
 * release. tokens.css animates it through data-press / data-pressed.
 */
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

/**
 * reveal fades and lifts an element in when it first enters the screen. Pass
 * the item index for a short stagger (30ms steps, at most 8 items).
 */
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
			// Clear the delay once in, so later hovers are not slowed.
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

/** safeTransitionName keeps a name to the characters CSS accepts as an ident. */
export function safeTransitionName(name: string): string {
	const clean = name.replace(/[^a-zA-Z0-9_-]/g, '-');
	return /^[a-zA-Z_]/.test(clean) ? clean : `vt-${clean}`;
}

/** viewTransitionName names an element so a page change can morph it (poster → hero). */
export const viewTransitionName: Action<HTMLElement, string | undefined | null> = (node, name) => {
	const set = (n: string | undefined | null) => {
		if (n) node.style.setProperty('view-transition-name', safeTransitionName(n));
		else node.style.removeProperty('view-transition-name');
	};
	set(name);
	return { update: set, destroy: () => set(null) };
};

type TransitionDoc = { startViewTransition?: (cb: () => Promise<void> | void) => unknown };

function defaultDoc(): TransitionDoc | undefined {
	return typeof document === 'undefined' ? undefined : (document as unknown as TransitionDoc);
}

/** canViewTransition says whether page changes may animate here. */
export function canViewTransition(doc: TransitionDoc | undefined = defaultDoc(), reduce = reducedMotion()): boolean {
	return !reduce && typeof doc?.startViewTransition === 'function';
}

type PageChange = { complete: Promise<void>; from?: { url: URL } | null; to?: { url: URL } | null };

/**
 * navigateWithTransition wraps a SvelteKit page change in a View Transition.
 * Use from onNavigate: `onNavigate((n) => navigateWithTransition(n))`.
 * Without support (or with reduced motion) the change happens plainly.
 */
export function navigateWithTransition(navigation: PageChange, doc: TransitionDoc | undefined = defaultDoc()): Promise<void> | undefined {
	if (!canViewTransition(doc)) return;
	// Same page, other query (filters, tabs): no page animation.
	if (navigation.from && navigation.to && navigation.from.url.pathname === navigation.to.url.pathname) return;
	const start = doc!.startViewTransition!;
	return new Promise((resolve) => {
		start.call(doc, async () => {
			resolve();
			await navigation.complete;
		});
	});
}

// The full-screen trailer's shared-element motion (Theater.svelte): one
// grow and one shrink, a spring-like ease-out that settles without bounce
// (tokens.css --ease-out), the controls fading in after and out before.
export const THEATER_GROW_MS = 420;
export const THEATER_SHRINK_MS = 380;
export const THEATER_CHROME_MS = 150;
export const THEATER_FADE_MS = 200;
export const THEATER_EASE = 'cubic-bezier(.22, 1, .36, 1)';

export interface Box {
	left: number;
	top: number;
	width: number;
	height: number;
}

/** containBox is where a picture of this aspect sits when fitted (object-fit: contain) into the window. */
export function containBox(aspect: number, vw: number, vh: number): Box {
	const a = aspect > 0 && Number.isFinite(aspect) ? aspect : 16 / 9;
	const width = Math.min(vw, vh * a);
	const height = width / a;
	return { left: (vw - width) / 2, top: (vh - height) / 2, width, height };
}

/**
 * coverFrom is the transform that makes the full-screen picture (fitted to
 * the window) cover a small rect, one uniform scale so it never stretches;
 * clipFrom is the clip that shows only that rect. Together they are the
 * first frame of the grow and the last frame of the shrink.
 */
export function coverFrom(rect: Box, aspect: number, vw: number, vh: number): string {
	const c = containBox(aspect, vw, vh);
	const s = Math.max(rect.width / c.width, rect.height / c.height);
	const dx = rect.left + rect.width / 2 - (c.left + c.width / 2);
	const dy = rect.top + rect.height / 2 - (c.top + c.height / 2);
	return `translate(${round(dx)}px, ${round(dy)}px) scale(${round(s, 4)})`;
}

export function clipFrom(rect: Box, vw: number, vh: number, radius: number): string {
	const t = Math.max(0, rect.top);
	const l = Math.max(0, rect.left);
	const r = Math.max(0, vw - rect.left - rect.width);
	const b = Math.max(0, vh - rect.top - rect.height);
	return `inset(${round(t)}px ${round(r)}px ${round(b)}px ${round(l)}px round ${radius}px)`;
}

function round(n: number, places = 2): number {
	const f = 10 ** places;
	return Math.round(n * f) / f;
}
