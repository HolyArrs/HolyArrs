// Trailer autoplay rules: which video, whether it may play here, and the delays.

import { t } from '#lib/core/i18n/i18n.svelte.ts';
export interface Video {
	key: string;
	name?: string;
	type: string;
	official?: boolean;
	language?: string;
}

export interface Conditions {
	setting: boolean;
	kid: boolean;
	reducedMotion: boolean;
	pref?: AutoplayPref;
	saveData?: boolean;
	effectiveType?: string;
	phone?: boolean;
	/** The minimal look is on in this browser. */
	minimal?: boolean;
}

export type AutoplayPref = 'auto' | 'always' | 'never';

const youTubeKey = /^[A-Za-z0-9_-]{6,20}$/;
const AUTOPLAY_KEY = 'holyarrs.trailer.autoplay';
const SOUND_KEY = 'holyarrs.trailer.sound';

// Delays in ms before a hero or hover trailer starts.
export const HERO_DELAY = 1500;
export const HOVER_DELAY = 800;

// Trailer beats Teaser, then official, then own language, then English; clips and odd keys never play.
export function pickTrailer(videos: Video[] | null | undefined, lang = 'en'): Video | null {
	const want = lang.toLowerCase().split('-')[0];
	let best: Video | null = null;
	let bestScore = -1;
	for (const v of videos ?? []) {
		if (!youTubeKey.test(v.key ?? '')) continue;
		let s = 0;
		if (v.type === 'Trailer') s += 100;
		else if (v.type !== 'Teaser') continue;
		if (v.official) s += 10;
		const l = (v.language ?? '').toLowerCase();
		if (want && l === want) s += 5;
		else if (l === 'en' || l === '') s += 2;
		if (s > bestScore) {
			best = v;
			bestScore = s;
		}
	}
	return best;
}

// Uses the no-cookie host.
export function embedUrl(key: string, origin?: string, repeat = false): string {
	const q = new URLSearchParams({ autoplay: '1', mute: '1', controls: '0', playsinline: '1', loop: '1', modestbranding: '1', rel: '0', enablejsapi: '1' });
	if (origin) q.set('origin', origin);
	// The embed only repeats a single video when playlist names it.
	if (repeat) q.set('playlist', key);
	return `https://www.youtube-nocookie.com/embed/${encodeURIComponent(key)}?${q}`;
}

export function playingWord(kind: 'movie' | 'show' | 'artist', what?: string): string {
	if (kind === 'artist') return t('core.trailer.musicVideo');
	return what || t('core.trailer.trailer');
}

// Main backdrop first, then more, no repeats.
export function heroPictures(main: string | null | undefined, more: string[] | null | undefined, max = 4): string[] {
	const out: string[] = [];
	for (const u of [main, ...(more ?? [])]) {
		if (u && !out.includes(u) && out.length < max) out.push(u);
	}
	return out;
}

// Per-device choice: auto follows system settings, always ignores them, never turns trailers off.
export function autoplayPref(): AutoplayPref {
	try {
		const v = localStorage.getItem(AUTOPLAY_KEY);
		return v === 'always' || v === 'never' ? v : 'auto';
	} catch {
		return 'auto';
	}
}

export function setAutoplayPref(v: AutoplayPref) {
	try {
		if (v === 'auto') localStorage.removeItem(AUTOPLAY_KEY);
		else localStorage.setItem(AUTOPLAY_KEY, v);
	} catch {
		// storage can be blocked; the choice then lasts this page only
	}
}

// Autoplay needs every condition to allow it; 'always' skips only the system ones, never the minimal look.
export function mayAutoplay(c: Conditions): boolean {
	const pref = c.pref ?? 'auto';
	if (!c.setting || c.kid || c.minimal || pref === 'never') return false;
	if (pref === 'always') return true;
	if (c.reducedMotion || c.saveData || c.phone) return false;
	return !['slow-2g', '2g', '3g'].includes(c.effectiveType ?? '');
}

// Without a window (server render) assume reduced motion so nothing plays.
export function browserConditions(): Pick<Conditions, 'reducedMotion' | 'saveData' | 'effectiveType' | 'phone'> {
	if (typeof window === 'undefined') return { reducedMotion: true };
	const conn = (navigator as Navigator & { connection?: { saveData?: boolean; effectiveType?: string } }).connection;
	return {
		reducedMotion: window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false,
		saveData: !!conn?.saveData,
		effectiveType: conn?.effectiveType,
		phone: (window.matchMedia?.('(max-width: 767px)').matches ?? false) || !(window.matchMedia?.('(hover: hover)').matches ?? true)
	};
}

// Hover cards only exist for a precise hovering pointer.
export function finePointer(): boolean {
	return typeof window !== 'undefined' && (window.matchMedia?.('(hover: hover) and (pointer: fine)').matches ?? false);
}

// A card under a resting pointer must not grow because the page scrolled under it:
// after a scroll, hover waits until the pointer itself moves.
let lastScroll = -Infinity;
let movedSinceScroll = true;
if (typeof window !== 'undefined') {
	window.addEventListener('scroll', () => ((lastScroll = performance.now()), (movedSinceScroll = false)), { passive: true, capture: true });
	window.addEventListener('pointermove', (e) => (e.movementX || e.movementY) && (movedSinceScroll = true), { passive: true, capture: true });
}

// 150 ms covers scroll momentum events that arrive just after the last one.
export function scrollHold(): boolean {
	return !movedSinceScroll || performance.now() - lastScroll < 150;
}

// Calling start again restarts the wait; cancel is always safe.
export function dwell(ms: number, fire: () => void, timers: { set: (f: () => void, ms: number) => ReturnType<typeof setTimeout>; clear: (t: ReturnType<typeof setTimeout>) => void } = { set: (f, ms) => setTimeout(f, ms), clear: (t) => clearTimeout(t) }) {
	let t: ReturnType<typeof setTimeout> | null = null;
	return {
		start() {
			if (t !== null) timers.clear(t);
			t = timers.set(() => {
				t = null;
				fire();
			}, ms);
		},
		cancel() {
			if (t !== null) timers.clear(t);
			t = null;
		},
		get pending() {
			return t !== null;
		}
	};
}

// One video at a time: a new player stops the one before it.
let playing: { id: symbol; stop: () => void } | null = null;

// Starting a player stops whichever one held the slot before.
export function claimPlayer(stop: () => void): () => void {
	const id = Symbol('player');
	if (playing) playing.stop();
	playing = { id, stop };
	return () => {
		if (playing?.id === id) playing = null;
	};
}

// Remembers a sound-on choice so the next trailer does not start muted.
export function soundOn(): boolean {
	try {
		return localStorage.getItem(SOUND_KEY) === 'on';
	} catch {
		return false;
	}
}

export function setSoundOn(on: boolean) {
	try {
		if (on) localStorage.setItem(SOUND_KEY, 'on');
		else localStorage.removeItem(SOUND_KEY);
	} catch {
		// storage can be blocked; not remembered
	}
}

// Keeps the card inside the viewport and bounds.
export function placeCard(
	anchor: { left: number; top: number; width: number; height: number },
	w: number,
	h: number,
	view: { width: number; height: number },
	margin = 8,
	bounds?: { left: number; right: number }
) {
	// Centre on the poster; at a row's edge, line up with the poster's outer edge instead.
	const lo = Math.max(margin, bounds?.left ?? margin);
	const hi = Math.min(view.width - margin, bounds?.right ?? view.width - margin);
	const cx = anchor.left + anchor.width / 2;
	const cy = anchor.top + anchor.height / 2;
	let left = cx - w / 2;
	if (left < lo) left = Math.max(anchor.left, lo);
	else if (left + w > hi) left = Math.min(anchor.left + anchor.width, hi) - w;
	const top = Math.min(Math.max(cy - h / 2, margin), Math.max(margin, view.height - h - margin));
	return { left: Math.round(left), top: Math.round(top) };
}
