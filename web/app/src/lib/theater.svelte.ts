// The full-screen trailer: one at a time, opened by clicking a playing
// trailer (hero, hover card or the More info panel). Closing it hands the
// title to whoever listens (Explore opens its More info panel), so the
// trailer flows into the details, Netflix style.
import type { TrailerSource } from './autoplay.svelte.ts';

export type TheaterFrom = 'hero' | 'hover' | 'panel';
export interface TheaterOpen {
	kind: 'movie' | 'show';
	tmdb: string;
	title: string;
	source: TrailerSource;
	/** Seconds already played in the small player; the big one carries on from there. */
	time: number;
	/** Where the small player sat, so the big one grows from it. */
	rect: { left: number; top: number; width: number; height: number } | null;
	from: TheaterFrom;
	/** Focus goes back here on close when nothing else took it (else to what had focus at open). */
	returnTo?: HTMLElement | null;
}

export const theater = $state<{ now: TheaterOpen | null }>({ now: null });

let exitTo: ((o: TheaterOpen) => void) | null = null;

/** onTheaterExit sets what happens once the full-screen trailer closes. */
export function onTheaterExit(fn: (o: TheaterOpen) => void): () => void {
	exitTo = fn;
	return () => {
		if (exitTo === fn) exitTo = null;
	};
}

export function openTheater(o: TheaterOpen) {
	const was = document.activeElement instanceof HTMLElement && document.activeElement !== document.body ? document.activeElement : null;
	theater.now = { ...o, returnTo: o.returnTo ?? was };
}

export function closeTheater() {
	const o = theater.now;
	theater.now = null;
	if (o) exitTo?.(o);
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
