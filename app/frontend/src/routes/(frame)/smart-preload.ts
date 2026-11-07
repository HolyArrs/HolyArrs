import { proxied } from '#lib/core/tmdb-image.ts';

// Smart preloading, one place for the rules every fetch-ahead follows
// (speed plan 2026-10-28, item 10):
// - hover or focus a card for 150 ms: its page's data and main picture;
// - a row scrolls into view: the next row's pictures;
// - a page has settled: on idle, the code of the likely next pages;
// - at most 4 at once, and only on a fast connection (4g, no data saver);
// - only our own addresses: never a picture or page on another host, so a
//   fetch ahead can never make an outside call from the visitor's browser.
// The page data itself goes through music-prefetch.ts (api.warm), so a page
// finds it in the shared answers.

/** A pointer or focus rests this long on a card before its page is fetched. */
export const DWELL_MS = 150;
/** Fetches ahead running at once. */
export const MAX_AT_ONCE = 4;
/** Pictures of the next row fetched when a row comes into view. */
export const NEXT_ROW_PICTURES = 8;

interface NetInfo {
	effectiveType?: string;
	saveData?: boolean;
}

/** fastEnough says the connection is worth fetching ahead on: 4g and no data saver.
 * A browser without the Network Information API (Safari, Firefox) is taken as fast. */
export function fastEnough(conn: NetInfo | undefined | null = typeof navigator === 'undefined' ? null : (navigator as Navigator & { connection?: NetInfo }).connection): boolean {
	if (!conn) return true;
	if (conn.saveData) return false;
	return !conn.effectiveType || conn.effectiveType === '4g';
}

/** ownAddress says url is served by this app (same origin): the only kind fetched ahead. */
export function ownAddress(url: string | null | undefined, origin: string = typeof location === 'undefined' ? '' : location.origin): boolean {
	if (!url || url.startsWith('data:') || url.startsWith('blob:')) return false;
	if (url.startsWith('/') && !url.startsWith('//')) return true;
	try {
		return !!origin && new URL(url).origin === origin;
	} catch {
		return false;
	}
}

/** createLimiter runs tasks at most max at once, in the order given; the rest wait. */
export function createLimiter(max = MAX_AT_ONCE) {
	let running = 0;
	const queue: (() => void)[] = [];
	const pump = () => {
		while (running < max && queue.length) (queue.shift() as () => void)();
	};
	return {
		run<T>(task: () => Promise<T>): Promise<T> {
			return new Promise<T>((resolve, reject) => {
				queue.push(() => {
					running++;
					task()
						.then(resolve, reject)
						.finally(() => {
							running--;
							pump();
						});
				});
				pump();
			});
		},
		running: () => running,
		waiting: () => queue.length
	};
}

const limiter = createLimiter();
const pictures = new Set<string>();

/** preloadPicture fetches one of our own pictures into the browser cache (once). */
export function preloadPicture(src: string | null | undefined, load: (src: string) => Promise<void> = imageLoad): Promise<void> {
	if (!src || pictures.has(src) || !ownAddress(src) || !fastEnough()) return Promise.resolve();
	pictures.add(src);
	return limiter.run(() => load(src)).catch(() => void pictures.delete(src));
}

function imageLoad(src: string): Promise<void> {
	return new Promise((resolve, reject) => {
		const img = new Image();
		img.decoding = 'async';
		img.onload = () => resolve();
		img.onerror = () => reject(new Error('picture'));
		img.src = proxied(src);
	});
}

/** mainPicture is the picture a page answer leads with, when it is one of ours. */
export function mainPicture(answer: unknown): string | null {
	if (!answer || typeof answer !== 'object') return null;
	const a = answer as Record<string, unknown>;
	for (const k of ['backdrop', 'image', 'poster', 'photo']) {
		const v = a[k];
		if (typeof v === 'string' && ownAddress(v)) return v;
	}
	const art = a.artwork;
	if (Array.isArray(art) && typeof art[0] === 'string' && ownAddress(art[0])) return art[0];
	return null;
}

/** rowPictures lists a row's picture addresses (ours only), first ones first. */
export function rowPictures(row: Element, n = NEXT_ROW_PICTURES): string[] {
	const out: string[] = [];
	for (const img of row.querySelectorAll<HTMLImageElement>('img')) {
		const src = img.currentSrc || img.getAttribute('src') || img.dataset.src || '';
		if (src && ownAddress(src) && !out.includes(src)) out.push(src);
		if (out.length >= n) break;
	}
	return out;
}

/** installRowAhead: when a row ([data-row]) comes into view, the next row's pictures are fetched. */
export function installRowAhead(root: ParentNode = document): () => void {
	if (typeof IntersectionObserver === 'undefined') return () => {};
	const seen = new WeakSet<Element>();
	const io = new IntersectionObserver(
		(entries) => {
			if (!fastEnough()) return;
			for (const e of entries) {
				if (!e.isIntersecting || seen.has(e.target)) continue;
				seen.add(e.target);
				const rows = [...root.querySelectorAll('[data-row]')];
				const next = rows[rows.indexOf(e.target) + 1];
				if (next) for (const src of rowPictures(next)) void preloadPicture(src);
			}
		},
		{ rootMargin: '0px 0px 200px 0px' }
	);
	const watch = () => root.querySelectorAll('[data-row]').forEach((el) => io.observe(el));
	watch();
	// Rows arrive after the page opens: new ones are watched as they appear.
	const mo = typeof MutationObserver === 'undefined' ? null : new MutationObserver(watch);
	if (mo && root instanceof Node) mo.observe(root, { childList: true, subtree: true });
	return () => {
		io.disconnect();
		mo?.disconnect();
	};
}

/** whenIdle runs fn when the browser is idle (or after a short pause where there is no idle callback). */
export function whenIdle(fn: () => void, timeout = 2000): () => void {
	const g = globalThis as typeof globalThis & { requestIdleCallback?: (cb: () => void, o?: { timeout: number }) => number; cancelIdleCallback?: (id: number) => void };
	if (g.requestIdleCallback) {
		const id = g.requestIdleCallback(fn, { timeout });
		return () => g.cancelIdleCallback?.(id);
	}
	const t = setTimeout(fn, 300);
	return () => clearTimeout(t);
}

/**
 * idlePages fetches ahead the code of the likely next pages (the side menu's
 * places) once a page has settled: on idle, at most MAX_AT_ONCE at a time, fast
 * connections only, each path once. preload is SvelteKit's preloadCode, which
 * loads the page's own scripts and nothing else.
 */
export function idlePages(paths: string[], preload: (path: string) => Promise<unknown>, done: Set<string> = idleDone): () => void {
	if (!fastEnough()) return () => {};
	return whenIdle(() => {
		for (const p of paths) {
			if (done.has(p) || !ownAddress(p)) continue;
			done.add(p);
			void limiter.run(() => preload(p)).catch(() => done.delete(p));
		}
	});
}
const idleDone = new Set<string>();
