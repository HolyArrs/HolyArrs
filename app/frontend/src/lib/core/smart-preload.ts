// Picture preloading for the app's own images: slow-link guard, small queue, idle and next-row warming.
import { proxied } from '#lib/core/tmdb-image.ts';

interface NetInfo {
	effectiveType?: string;
	saveData?: boolean;
}

// Pointer must rest this long before a hover counts.
export const DWELL_MS = 150;
// A few at a time, so warming never competes with the page itself.
export const MAX_AT_ONCE = 4;
export const NEXT_ROW_PICTURES = 8;
const limiter = createLimiter();
const pictures = new Set<string>();
const idleDone = new Set<string>();

// Skip warming on data saver or slow links.
export function fastEnough(conn: NetInfo | undefined | null = typeof navigator === 'undefined' ? null : (navigator as Navigator & { connection?: NetInfo }).connection): boolean {
	if (!conn) return true;
	if (conn.saveData) return false;
	return !conn.effectiveType || conn.effectiveType === '4g';
}

// Only same-origin paths are safe to warm; data and blob urls need no fetch.
export function ownAddress(url: string | null | undefined, origin: string = typeof location === 'undefined' ? '' : location.origin): boolean {
	if (!url || url.startsWith('data:') || url.startsWith('blob:')) return false;
	if (url.startsWith('/') && !url.startsWith('//')) return true;
	try {
		return !!origin && new URL(url).origin === origin;
	} catch {
		return false;
	}
}

// Runs at most max tasks at once; the rest wait in order.
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

// A failed picture is forgotten so a later try can run again.
export function preloadPicture(src: string | null | undefined, load: (src: string) => Promise<void> = imageLoad): Promise<void> {
	if (!src || pictures.has(src) || !ownAddress(src) || !fastEnough()) return Promise.resolve();
	pictures.add(src);
	return limiter.run(() => load(src)).catch(() => void pictures.delete(src));
}

// First usable picture in an answer, trying the biggest art first.
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

export function rowPictures(row: Element, n = NEXT_ROW_PICTURES): string[] {
	const out: string[] = [];
	for (const img of row.querySelectorAll<HTMLImageElement>('img')) {
		const src = img.currentSrc || img.getAttribute('src') || img.dataset.src || '';
		if (src && ownAddress(src) && !out.includes(src)) out.push(src);
		if (out.length >= n) break;
	}
	return out;
}

// Warms the pictures of the next row once the current one scrolls into view.
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
	const mo = typeof MutationObserver === 'undefined' ? null : new MutationObserver(watch);
	if (mo && root instanceof Node) mo.observe(root, { childList: true, subtree: true });
	return () => {
		io.disconnect();
		mo?.disconnect();
	};
}

// Falls back to a timer where idle callbacks do not exist.
export function whenIdle(fn: () => void, timeout = 2000): () => void {
	const g = globalThis as typeof globalThis & { requestIdleCallback?: (cb: () => void, o?: { timeout: number }) => number; cancelIdleCallback?: (id: number) => void };
	if (g.requestIdleCallback) {
		const id = g.requestIdleCallback(fn, { timeout });
		return () => g.cancelIdleCallback?.(id);
	}
	const t = setTimeout(fn, 300);
	return () => clearTimeout(t);
}

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

// Goes through the proxied address so it fills the same cache the page uses.
function imageLoad(src: string): Promise<void> {
	return new Promise((resolve, reject) => {
		const img = new Image();
		img.decoding = 'async';
		img.onload = () => resolve();
		img.onerror = () => reject(new Error('picture'));
		img.src = proxied(src);
	});
}
