// Trailer autoplay: which video, whether it may play here, and the delays.
// Layer 1 (moving pictures) always shows; Layer 2 (a muted trailer) plays
// only when everything below says yes, and only one at a time.

export interface Video {
	key: string;
	name?: string;
	type: string;
	official?: boolean;
	language?: string;
}

const youTubeKey = /^[A-Za-z0-9_-]{6,20}$/;

/**
 * pickTrailer chooses the trailer: a Trailer before a Teaser, official
 * first, the person's language before English before others, keeping
 * TMDB's order otherwise. Clips and odd keys never play.
 */
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

/** The privacy-enhanced embed address (muted autoplay, no controls). */
export function embedUrl(key: string, origin?: string, repeat = false): string {
	const q = new URLSearchParams({ autoplay: '1', mute: '1', controls: '0', playsinline: '1', loop: '1', modestbranding: '1', rel: '0', enablejsapi: '1' });
	if (origin) q.set('origin', origin);
	// loop=1 repeats a single video only with playlist=<key>; without it the video ends.
	if (repeat) q.set('playlist', key);
	return `https://www.youtube-nocookie.com/embed/${encodeURIComponent(key)}?${q}`;
}

/** The hero's pictures: the main backdrop first, then up to 3 more, no repeats. */
export function heroPictures(main: string | null | undefined, more: string[] | null | undefined, max = 4): string[] {
	const out: string[] = [];
	for (const u of [main, ...(more ?? [])]) {
		if (u && !out.includes(u) && out.length < max) out.push(u);
	}
	return out;
}

export interface Conditions {
	/** The owner's "Autoplay trailers" setting. */
	setting: boolean;
	kid: boolean;
	reducedMotion: boolean;
	/** This device's choice (autoplayPref); auto when left out. */
	pref?: AutoplayPref;
	saveData?: boolean;
	/** navigator.connection.effectiveType */
	effectiveType?: string;
	/** A phone (touch only, or narrower than a tablet): the trailer waits for a tap. */
	phone?: boolean;
}

/** mayAutoplay says whether Layer 2 may start here; otherwise Layer 1 stays. */
export type AutoplayPref = 'auto' | 'always' | 'never';
const AUTOPLAY_KEY = 'holyarrs.trailer.autoplay';

/** This device's trailer choice: auto follows the system (reduced motion, data saver), always ignores it, never turns trailers off. */
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
		/* private mode: the choice lasts this page only */
	}
}

export function mayAutoplay(c: Conditions): boolean {
	const pref = c.pref ?? 'auto';
	if (!c.setting || c.kid || pref === 'never') return false;
	if (pref === 'always') return true;
	if (c.reducedMotion || c.saveData || c.phone) return false;
	return !['slow-2g', '2g', '3g'].includes(c.effectiveType ?? '');
}

/** What this browser says about motion and data. */
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

/** True on a desktop pointer (hover cards only there). */
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

/** True while the page scrolls, and after it until the pointer moves: hover cards stay shut. */
export function scrollHold(): boolean {
	return !movedSinceScroll || performance.now() - lastScroll < 150;
}

/**
 * dwell runs fire after ms unless cancelled first: the hover and in-view
 * delays. start again restarts; cancel is safe to call any time.
 */
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

/** claim makes this the only playing video; the returned release frees the slot. */
export function claimPlayer(stop: () => void): () => void {
	const id = Symbol('player');
	if (playing) playing.stop();
	playing = { id, stop };
	return () => {
		if (playing?.id === id) playing = null;
	};
}

/** For tests: whether some player holds the slot. */
export function playerBusy(): boolean {
	return playing !== null;
}

const SOUND_KEY = 'holyarrs.trailer.sound';

/** Whether the person turned trailer sound on earlier in this browser session. */
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
		/* private mode: not remembered */
	}
}

export const HERO_DELAY = 1500;
export const HOVER_DELAY = 800;

/** Where a hover card of size w×h opens, kept inside the viewport (8px margin). */
export function placeCard(
	anchor: { left: number; top: number; width: number; height: number },
	w: number,
	h: number,
	view: { width: number; height: number },
	margin = 8,
	bounds?: { left: number; right: number }
) {
	// Netflix-style: grow from the poster itself, centred on it; at a row's
	// edge, line up with the poster's outer edge instead of sliding away.
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
