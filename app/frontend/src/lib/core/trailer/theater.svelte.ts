// State and helpers for the single full-screen trailer.
import type { TrailerSource } from './autoplay.svelte.ts';

export type TheaterFrom = 'hero' | 'hover' | 'panel';
export interface TheaterOpen {
	// For an artist, tmdb holds the artist's MusicBrainz id.
	kind: 'movie' | 'show' | 'artist';
	tmdb: string;
	title: string;
	what?: string;
	source: TrailerSource;
	fallbacks?: TrailerSource[];
	time: number;
	rect: { left: number; top: number; width: number; height: number } | null;
	from: TheaterFrom;
	muted?: boolean;
	// Shown until the big player has its first frame, to avoid a black flash.
	still?: HTMLCanvasElement | null;
	returnTo?: HTMLElement | null;
}

export interface TheaterBack {
	kind: 'movie' | 'show' | 'artist';
	tmdb: string;
	time: number;
	muted: boolean;
	ended: boolean;
	from?: TheaterFrom;
}
export interface TheaterHandoff {
	source: TrailerSource;
	time: number;
	rect: DOMRect;
	muted: boolean;
	still: HTMLCanvasElement | null;
}
export function fromHandoff(o: TheaterHandoff) {
	return { source: o.source, time: o.time, rect: { left: o.rect.left, top: o.rect.top, width: o.rect.width, height: o.rect.height }, muted: o.muted, still: o.still };
}

// back lets the hero under a closing trailer carry on from the same second; landing
// means the player closes over a page change, so no page transition runs.
export const theater = $state<{ now: TheaterOpen | null; back: TheaterBack | null; landing: boolean }>({ now: null, back: null, landing: false });

let resumed: (() => void) | null = null;
// Resolves after ms at the latest, so a hero that never resumes cannot hang the close.
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
// Only the hero for the same title takes it, and only once.
export function takeBack(kind: string | undefined, tmdb: string | null | undefined): TheaterBack | null {
	const b = theater.back;
	if (!b || b.kind !== kind || b.tmdb !== tmdb) return null;
	theater.back = null;
	return b;
}
export function resumedPlaying() {
	resumed?.();
}

let exitTo: ((o: TheaterOpen) => unknown) | null = null;

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

// Runs while the player still covers the window so the page change is hidden.
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

export function closeTheater() {
	const o = theater.now;
	theater.now = null;
	// Return focus to the opener unless the exit already moved it.
	requestAnimationFrame(() => {
		const a = document.activeElement;
		if (o?.returnTo?.isConnected && (!a || a === document.body)) o.returnTo.focus();
	});
}

// Counted so the panel and the trailer can both hold the lock; the scrollbar's width is kept to avoid a jump.
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
