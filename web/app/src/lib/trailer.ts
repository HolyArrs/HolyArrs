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
	saveData?: boolean;
	/** navigator.connection.effectiveType */
	effectiveType?: string;
}

/** mayAutoplay says whether Layer 2 may start here; otherwise Layer 1 stays. */
export function mayAutoplay(c: Conditions): boolean {
	if (!c.setting || c.kid || c.reducedMotion || c.saveData) return false;
	return !['slow-2g', '2g', '3g'].includes(c.effectiveType ?? '');
}

/** What this browser says about motion and data. */
export function browserConditions(): Pick<Conditions, 'reducedMotion' | 'saveData' | 'effectiveType'> {
	if (typeof window === 'undefined') return { reducedMotion: true };
	const conn = (navigator as Navigator & { connection?: { saveData?: boolean; effectiveType?: string } }).connection;
	return {
		reducedMotion: window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false,
		saveData: !!conn?.saveData,
		effectiveType: conn?.effectiveType
	};
}

/** True on a desktop pointer (hover cards only there). */
export function finePointer(): boolean {
	return typeof window !== 'undefined' && (window.matchMedia?.('(hover: hover) and (pointer: fine)').matches ?? false);
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

export const HERO_DELAY = 2500;
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
