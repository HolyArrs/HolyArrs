// The full-screen trailer: one at a time, opened by clicking a playing
// trailer (hero, hover card or the More info panel). When the trailer ends
// it hands the title to whoever listens (Explore opens the title page), so
// it flows into the details; Esc and Close only close it.
import type { TrailerSource } from './autoplay.svelte.ts';

export type TheaterFrom = 'hero' | 'hover' | 'panel';
export interface TheaterOpen {
	kind: 'movie' | 'show';
	tmdb: string;
	title: string;
	source: TrailerSource;
	/** Tried in order when the source cannot play (the relayed stream refused, say), before the player gives up. */
	fallbacks?: TrailerSource[];
	/** Seconds already played in the small player; the big one carries on from there. */
	time: number;
	/** Where the small player sat, so the big one grows from it. */
	rect: { left: number; top: number; width: number; height: number } | null;
	from: TheaterFrom;
	/** The small player's sound state; the big one keeps it (and hands it back). */
	muted?: boolean;
	/** The small player's last frame, shown until the big one has its first frame (no black frame). */
	still?: HTMLCanvasElement | null;
	/** Focus goes back here on close when nothing else took it (else to what had focus at open). */
	returnTo?: HTMLElement | null;
}

/**
 * now: the open trailer. back: what the closing trailer hands to the hero it
 * shrinks into (same title), so that hero carries on from the same second.
 * landing: the player is closing over a page change (no page transition then).
 */
export interface TheaterBack {
	kind: 'movie' | 'show';
	tmdb: string;
	time: number;
	muted: boolean;
	ended: boolean;
	/** Where the closing trailer was opened; a hero carries on only from its own (hero) or a hover card. */
	from?: TheaterFrom;
}
/** What a small player hands to the full-screen one when clicked. */
export interface TheaterHandoff {
	source: TrailerSource;
	time: number;
	rect: DOMRect;
	muted: boolean;
	still: HTMLCanvasElement | null;
}
/** theaterFrom turns a small player's hand-off into the open call's fields. */
export function fromHandoff(o: TheaterHandoff) {
	return { source: o.source, time: o.time, rect: { left: o.rect.left, top: o.rect.top, width: o.rect.width, height: o.rect.height }, muted: o.muted, still: o.still };
}

export const theater = $state<{ now: TheaterOpen | null; back: TheaterBack | null; landing: boolean }>({ now: null, back: null, landing: false });

let resumed: (() => void) | null = null;
/** waitForResume resolves when the hero under the player is playing again (or after ms). */
export function waitForResume(ms: number): Promise<void> {
	return new Promise((done) => {
		const t = setTimeout(finish, ms);
		function finish() {
			clearTimeout(t);
			if (resumed === finish) resumed = null;
			done();
		}
		resumed = finish;
	});
}
/** takeBack is called by a hero for its title: it takes the hand-back (once). */
export function takeBack(kind: string | undefined, tmdb: string | null | undefined): TheaterBack | null {
	const b = theater.back;
	if (!b || b.kind !== kind || b.tmdb !== tmdb) return null;
	theater.back = null;
	return b;
}
/** resumedPlaying: the hero that took the hand-back shows moving pictures again. */
export function resumedPlaying() {
	resumed?.();
}

let exitTo: ((o: TheaterOpen) => unknown) | null = null;

/** onTheaterExit sets what happens once the full-screen trailer closes. */
export function onTheaterExit(fn: (o: TheaterOpen) => unknown): () => void {
	exitTo = fn;
	return () => {
		if (exitTo === fn) exitTo = null;
	};
}

export function openTheater(o: TheaterOpen) {
	const was = document.activeElement instanceof HTMLElement && document.activeElement !== document.body ? document.activeElement : null;
	theater.now = { ...o, returnTo: o.returnTo ?? was };
}

/**
 * runExit runs the exit (the title page opens) while the player still covers
 * the window, and resolves once the new page is in place. False: no exit set.
 */
export async function runExit(o: TheaterOpen): Promise<boolean> {
	if (!exitTo) return false;
	theater.landing = true;
	try {
		await exitTo(o);
	} finally {
		theater.landing = false;
	}
	return true;
}

/** closeTheater clears the full-screen trailer once it has shrunk away (Theater.svelte runs the exit first). */
export function closeTheater() {
	const o = theater.now;
	theater.now = null;
	// Focus back on the card or button that opened it, unless the exit (a panel, a page) took it.
	requestAnimationFrame(() => {
		const a = document.activeElement;
		if (o?.returnTo?.isConnected && (!a || a === document.body)) o.returnTo.focus();
	});
}

// Scroll lock behind the panel and the full-screen trailer (either or both):
// the page keeps its place and its width (the scrollbar's room is kept).
let holds = 0;
export function holdScroll(): () => void {
	const root = document.documentElement;
	if (holds++ === 0) {
		root.style.setProperty('--held-scrollbar', `${window.innerWidth - root.clientWidth}px`);
		root.classList.add('scroll-held');
	}
	let done = false;
	return () => {
		if (done) return;
		done = true;
		if (--holds === 0) {
			root.classList.remove('scroll-held');
			root.style.removeProperty('--held-scrollbar');
		}
	};
}
