// Artist and album pages open filled in: the data a music page asks first is fetched ahead when a mouse rests on a link to it (80 ms), when a finger touches one, and for the first cards on screen after a page opens.
import { appPicture, coverFor } from './music-pages.ts';
import { api } from '../core/api.ts';
import { DWELL_MS, fastEnough, mainPicture, preloadPicture } from '../core/smart-preload.ts';

type Get = (path: string) => Promise<unknown>;

const ID = '[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}';
const ARTIST = new RegExp(`/music/artist/(${ID})(?:[/?#]|$)`, 'i');
const ALBUM = new RegExp(`/music/album/(${ID})(?:[/?#]|$)`, 'i');
const TITLE = /\/title\/(movie|show)\/([1-9][0-9]*)(?:[/?#]|$)/;
const LIBRARY = /\/library\/([1-9][0-9]*)(?:[?#]|$)/;
const LINKS = 'a[href*="/music/artist/"], a[href*="/music/album/"], a[href*="/title/"], a[href*="/library/"]';

/** An answer fetched ahead is kept at least this long for its page. */
export const KEEP = 30_000;

/** A mouse or focus rests this long on a link before its page is fetched (smart-preload.ts). */
export const DWELL = DWELL_MS;

/** Cards on screen fetched ahead after a page opens. */
export const VIEW_CARDS = 6;

/** Fetches running at once; the rest wait their turn. */
export const IN_FLIGHT = 3;

/** Fetches started in any minute, hover and touch included. */
export const PER_MINUTE = 40;

/** An address fetched within this long is not fetched again. */
export const FRESH = 60_000;

/** The addresses a music page link's page asks first: an artist's full answer (it holds the core; the page asks the core only when the full one is slow) and an album's full answer (with its artist, as the page asks it). Others: none. */
export function musicPrefetchURLs(href: string | null | undefined): string[] {
	if (!href) return [];
	let m = ARTIST.exec(href);
	if (m) return [`/music/artists/${m[1]}?similar=0`];
	m = ALBUM.exec(href);
	if (!m) return [];
	const q = href.indexOf('?');
	const artist = (q < 0 ? '' : (new URLSearchParams(href.slice(q + 1).split('#')[0]).get('artist') ?? '')).toLowerCase();
	return artist ? [`/music/albums/${m[1].toLowerCase()}?artist=${artist}&similar=0`] : [];
}

/** The addresses any page link's page asks first: music pages as above, a title page's metadata and lookup (loadDetails), a Library title's own answer. Others: none. */
export function pagePrefetchURLs(href: string | null | undefined): string[] {
	if (!href) return [];
	const music = musicPrefetchURLs(href);
	if (music.length) return music;
	let m = TITLE.exec(href);
	if (m) return [`/metadata/${m[1]}/${m[2]}`, `${m[1] === 'movie' ? '/titles' : '/shows'}/lookup?term=${encodeURIComponent(`tmdb:${m[2]}`)}`];
	m = LIBRARY.exec(href);
	return m ? [`/titles/${m[1]}`] : [];
}

/** The address a page shows its main picture at, so the copy fetched ahead is the one it uses: an album's hero cover (versioned, at its slot size), an artist's versioned photo, other pages' main picture as is. */
export function pagePicture(path: string, answer: unknown): string | null {
	const main = mainPicture(answer);
	if (!main) return null;
	if (path.startsWith('/music/albums/')) return coverFor(appPicture(main), 260, undefined, true).src;
	if (path.startsWith('/music/artists/')) return appPicture(main);
	return main;
}

/** The fetch-ahead queue, separate for tests; installMusicPrefetch uses one for the app. */
export function createMusicPrefetch(get: Get = warmPage, now: () => number = () => Date.now()) {
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

	/** Queues a link's addresses; urgent ones (hover, touch) go first. Returns how many were new. */
	function want(href: string | null | undefined, urgent: boolean): number {
		const t = now();
		while (started.length && t - started[0] >= 60_000) started.shift();
		const fresh: string[] = [];
		for (const url of pagePrefetchURLs(href)) {
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
				if (!pagePrefetchURLs(href).length) continue;
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

let scans: ReturnType<typeof setTimeout>[] = [];

/** After each navigation: the first cards on screen, once the page has drawn and again once its rows have. */
export function prefetchMusicOnScreen(): void {
	if (typeof window === 'undefined') return;
	scans.forEach(clearTimeout);
	ahead.newPage();
	if (!fastEnough()) return;
	scans = [700, 2500].map((ms) => setTimeout(() => ahead.view(onScreen()), ms));
}

/** Forgets what was fetched ahead (sign-out); the shared answers are cleared by the client. */
export function forgetMusicPrefetch(): void {
	ahead.forget();
}

/** Hover (mouse, 150 ms), focus (150 ms) and touch on any music page link, for the whole app. Returns the cleanup. */
export function installMusicPrefetch(): () => void {
	let timer: ReturnType<typeof setTimeout> | undefined;
	const link = (e: Event) => (e.target as Element | null)?.closest?.('a[href]')?.getAttribute('href') ?? null;
	const over = (e: PointerEvent) => {
		if (e.pointerType !== 'mouse') return;
		clearTimeout(timer);
		const href = link(e);
		if (href && pagePrefetchURLs(href).length && fastEnough()) timer = setTimeout(() => ahead.hover(href), DWELL);
	};
	// Keyboard focus counts as a rest too.
	const focus = (e: FocusEvent) => {
		clearTimeout(timer);
		const href = link(e);
		if (href && pagePrefetchURLs(href).length && fastEnough()) timer = setTimeout(() => ahead.hover(href), DWELL);
	};
	// Leaving the link (not moving inside it) stops the wait.
	const out = (e: PointerEvent) => {
		const a = (e.target as Element | null)?.closest?.('a[href]');
		if (!a || !a.contains(e.relatedTarget as Node | null)) clearTimeout(timer);
	};
	const touch = (e: TouchEvent) => ahead.hover(link(e));
	window.addEventListener('pointerover', over, { capture: true, passive: true });
	window.addEventListener('touchstart', touch, { capture: true, passive: true });
	window.addEventListener('focusin', focus, { capture: true });
	window.addEventListener('pointerout', out, { capture: true, passive: true });
	return () => {
		window.removeEventListener('focusin', focus, { capture: true });
		window.removeEventListener('pointerout', out, { capture: true });
		clearTimeout(timer);
		scans.forEach(clearTimeout);
		window.removeEventListener('pointerover', over, { capture: true });
		window.removeEventListener('touchstart', touch, { capture: true });
	};
}

/** Fetches one address ahead; a Library title then fetches its metadata too, as its page will. */
function warmPage(path: string): Promise<unknown> {
	const answer = api.warm<{ kind?: string; ids?: { tmdb?: string } }>(path, KEEP);
	// The page's main picture too, when it is one of ours.
	answer.then((a) => void preloadPicture(pagePicture(path, a)), () => {});
	if (/^\/titles\/[0-9]+$/.test(path))
		answer.then(
			(t) => {
				const id = t?.ids?.tmdb;
				if ((t?.kind === 'movie' || t?.kind === 'show') && id) api.warm(`/metadata/${t.kind}/${encodeURIComponent(id)}`, KEEP).catch(() => {});
			},
			() => {}
		);
	return answer;
}

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
