// Artist and album pages open filled in (speed waves, 2026-10-27): the data a
// music page asks first is fetched ahead when a mouse rests on a link to it
// (80 ms), when a finger touches one, and for the first cards on screen after
// a page opens. The answers wait in api.get's shared answers (kept as long as
// the server's max-age, 60 s), at the exact address the page asks, so the
// page finds them there. A few at a time, with a cap per page and per minute.
import { api } from './api.ts';

const ID = '[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}';
const ARTIST = new RegExp(`/music/artist/(${ID})(?:[/?#]|$)`, 'i');
const ALBUM = new RegExp(`/music/album/(${ID})(?:[/?#]|$)`, 'i');
const LINKS = 'a[href*="/music/artist/"], a[href*="/music/album/"]';

/** A mouse rests this long on a link before its page is fetched. */
export const DWELL = 80;
/** Cards on screen fetched ahead after a page opens. */
export const VIEW_CARDS = 6;
/** Fetches running at once; the rest wait their turn. */
export const IN_FLIGHT = 3;
/** Fetches started in any minute, hover and touch included. */
export const PER_MINUTE = 40;
/** An address fetched within this long is not fetched again. */
export const FRESH = 60_000;

/** The addresses a music page link's page asks first: an artist's core and
 * full answer, an album's core answer (it needs the artist). Others: none. */
export function musicPrefetchURLs(href: string | null | undefined): string[] {
	if (!href) return [];
	let m = ARTIST.exec(href);
	if (m) return [`/music/artists/${m[1]}?part=core`, `/music/artists/${m[1]}?similar=0`];
	m = ALBUM.exec(href);
	if (!m) return [];
	const q = href.indexOf('?');
	const artist = (q < 0 ? '' : (new URLSearchParams(href.slice(q + 1).split('#')[0]).get('artist') ?? '')).toLowerCase();
	return artist ? [`/music/albums/${m[1].toLowerCase()}?part=core&artist=${artist}`] : [];
}

type Get = (path: string) => Promise<unknown>;

/** The fetch-ahead queue, on its own for tests; installMusicPrefetch uses one for the app. */
export function createMusicPrefetch(get: Get = (p) => api.get(p), now: () => number = () => Date.now()) {
	const asked = new Map<string, number>();
	const started: number[] = [];
	const queue: string[] = [];
	let running = 0;
	let viewed = 0;

	function pump() {
		while (running < IN_FLIGHT && queue.length) {
			const url = queue.shift() as string;
			running++;
			get(url)
				.catch(() => asked.delete(url))
				.finally(() => {
					running--;
					pump();
				});
		}
	}

	/** Queue a link's addresses; urgent ones (hover, touch) go first. Answers how many were new. */
	function want(href: string | null | undefined, urgent: boolean): number {
		const t = now();
		while (started.length && t - started[0] >= 60_000) started.shift();
		const fresh: string[] = [];
		for (const url of musicPrefetchURLs(href)) {
			const at = asked.get(url);
			if (at !== undefined && t - at < FRESH) {
				// Still waiting its turn: a hover moves it to the front.
				const i = urgent ? queue.indexOf(url) : -1;
				if (i > 0) queue.unshift(...queue.splice(i, 1));
				continue;
			}
			if (started.length >= PER_MINUTE) break;
			asked.set(url, t);
			started.push(t);
			fresh.push(url);
		}
		if (urgent) queue.unshift(...fresh);
		else queue.push(...fresh);
		pump();
		return fresh.length;
	}

	return {
		/** A link the pointer rests on or a finger touches. */
		hover: (href: string | null | undefined) => want(href, true),
		/** Links on screen, in order: the first ones until VIEW_CARDS cards this page. */
		view(hrefs: (string | null | undefined)[]) {
			for (const href of hrefs) {
				if (viewed >= VIEW_CARDS) return;
				if (!musicPrefetchURLs(href).length) continue;
				viewed++;
				want(href, false);
			}
		},
		/** A new page: its own cards on screen count again. */
		newPage() {
			viewed = 0;
		},
		/** Signed out or another account: fetch everything again when asked. */
		forget() {
			asked.clear();
			queue.length = 0;
		},
		pending: () => queue.length,
		running: () => running
	};
}

const ahead = createMusicPrefetch();

/** The links to music pages inside the screen, in page order. */
function onScreen(): string[] {
	const vh = innerHeight;
	const vw = innerWidth;
	const out: string[] = [];
	for (const a of document.querySelectorAll<HTMLAnchorElement>(LINKS)) {
		const b = a.getBoundingClientRect();
		if (b.width > 40 && b.height > 0 && b.top < vh && b.bottom > 0 && b.left < vw && b.right > 0) out.push(a.getAttribute('href') ?? '');
	}
	return out;
}

let scans: ReturnType<typeof setTimeout>[] = [];
/** After each navigation: the first cards on screen, once the page has drawn and again once its rows have. */
export function prefetchMusicOnScreen(): void {
	if (typeof window === 'undefined') return;
	scans.forEach(clearTimeout);
	ahead.newPage();
	if ((navigator as Navigator & { connection?: { saveData?: boolean } }).connection?.saveData) return;
	scans = [700, 2500].map((ms) => setTimeout(() => ahead.view(onScreen()), ms));
}

/** Forget what was fetched ahead (sign-out); the shared answers are cleared by the client. */
export function forgetMusicPrefetch(): void {
	ahead.forget();
}

/** Hover (mouse, 80 ms) and touch on any music page link, for the whole app. Returns the cleanup. */
export function installMusicPrefetch(): () => void {
	let timer: ReturnType<typeof setTimeout> | undefined;
	const link = (e: Event) => (e.target as Element | null)?.closest?.('a[href]')?.getAttribute('href') ?? null;
	const over = (e: PointerEvent) => {
		if (e.pointerType !== 'mouse') return;
		clearTimeout(timer);
		const href = link(e);
		if (href && musicPrefetchURLs(href).length) timer = setTimeout(() => ahead.hover(href), DWELL);
	};
	const touch = (e: TouchEvent) => ahead.hover(link(e));
	window.addEventListener('pointerover', over, { capture: true, passive: true });
	window.addEventListener('touchstart', touch, { capture: true, passive: true });
	return () => {
		clearTimeout(timer);
		scans.forEach(clearTimeout);
		window.removeEventListener('pointerover', over, { capture: true });
		window.removeEventListener('touchstart', touch, { capture: true });
	};
}
