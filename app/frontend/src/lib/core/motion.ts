// Calm motion. Svelte actions plus a page
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
	// Inside Settings the zoom, push and fade play instead (routes/settings/(home)/zoom.ts);
	// a View Transition would freeze the screen while the next page loads.
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

function round(n: number, places = 2): number {
	const f = 10 ** places;
	return Math.round(n * f) / f;
}

/** cubicBezier is a CSS cubic-bezier(x1, y1, x2, y2) timing function. */
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
/** The theater's ease as a function (THEATER_EASE). */
export const theaterEase = cubicBezier(0.22, 1, 0.36, 1);

/**
 * flipFrames are the keyframes of the grow, compositor only: the frame
 * (`outer`, clips) scales from the small rect to the window, and the picture
 * inside (`inner`) is counter-scaled so it keeps one uniform scale and
 * covers the frame the whole way. The ease is baked into the frames (play
 * them with linear timing), which keeps the counter-scale exact.
 */
export function flipFrames(from: Box, aspect: number, vw: number, vh: number, shrink = false, steps = 24, ease = theaterEase): { outer: string[]; inner: string[] } {
	const c = containBox(aspect, vw, vh);
	const s0 = Math.max(from.width / c.width, from.height / c.height);
	const ccx = c.left + c.width / 2;
	const ccy = c.top + c.height / 2;
	const outer: string[] = [];
	const inner: string[] = [];
	for (let i = 0; i <= steps; i++) {
		// The shrink eases out too: fast off the window, settling into the rect.
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

// Settings page changes: a card grows into its page and shrinks back (transform and
// opacity only, so every frame runs on the compositor), with a fade and a small
// settle on phones. Every animation can be cancelled or turned around mid-flight.

export const EASE = 'cubic-bezier(.22, 1, .36, 1)';
export const MS = {
	/** Card surface growing into the page, and shrinking back. */
	zoom: 420,
	zoomBack: 380,
	/** The page fading into a blank sheet before Back shrinks it. */
	cover: 120,
	/** The other cards stepping back or forward. */
	recede: 280,
	/** The card's picture fading while it travels. */
	artFade: 200,
	/** Page coming up under the grown card, and the grown card clearing. */
	reveal: 220,
	/** Phone push and pop. */
	push: 380,
	pop: 320,
	/** A page inside the same group. */
	swap: 200
} as const;

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

/** radiusFor keeps the sheet's corners close to the card's at the start and soft, not huge, at full size. */
export function radiusFor(card: Box, sheet: Box, cardRadius = 16): number {
	const s = sheet.width > 0 ? Math.min(1, card.width / sheet.width) : 1;
	return Math.round(cardRadius / Math.sqrt(Math.max(s, 0.05)));
}
