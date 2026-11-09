import type { FilmographyPage, FilmSort } from '#lib/ui/people.ts';
import { api } from '../core/api.ts';

// A person page opens filled in when the pointer has rested on that person's face: the first page of their filmography is fetched on hover and handed to the page.
const TTL = 60_000;
const DWELL = 120;
const SORT_KEY = 'holyarrs.people.sort';
const SORTS: FilmSort[] = ['oldest', 'newest', 'popular'];
const cache = new Map<string, { at: number; answer: Promise<FilmographyPage> }>();

/** The filmography order this browser last chose on a person page. */
export function peopleSort(): FilmSort {
	try {
		const v = localStorage.getItem(SORT_KEY) as FilmSort | null;
		return v && SORTS.includes(v) ? v : 'oldest';
	} catch {
		return 'oldest';
	}
}

/** The address of a person page's first answer (all roles). */
export function firstPageURL(id: string, sort: FilmSort): string {
	return `/people/${encodeURIComponent(id)}?role=all&sort=${sort}&page_size=100&page=1`;
}

/** Fetches a person's first page once, unless a fresh copy is already on its way. */
export function prefetchPerson(id: string, sort: FilmSort = peopleSort(), now = Date.now()) {
	const url = firstPageURL(id, sort);
	const hit = cache.get(url);
	if (hit && now - hit.at < TTL) return;
	const answer = api.get<FilmographyPage>(url);
	answer.catch(() => cache.delete(url));
	cache.set(url, { at: now, answer });
}

/** The prefetched answer for `url`, used once; null when there is none or it went stale. */
export function takePrefetched(url: string, now = Date.now()): Promise<FilmographyPage> | null {
	const hit = cache.get(url);
	if (!hit) return null;
	cache.delete(url);
	return now - hit.at < TTL ? hit.answer : null;
}

/** Prefetches while a mouse pointer rests on any link to a person page. Returns the cleanup. */
export function installPersonPrefetch(): () => void {
	let timer: ReturnType<typeof setTimeout> | undefined;
	const over = (e: PointerEvent) => {
		if (e.pointerType !== 'mouse') return;
		clearTimeout(timer);
		const a = (e.target as Element | null)?.closest?.('a[href]');
		const m = a?.getAttribute('href')?.match(/\/people\/([^/?#]+)/);
		if (m) timer = setTimeout(() => prefetchPerson(decodeURIComponent(m[1])), DWELL);
	};
	// The window's capture phase sees the pointer before anything else on the page.
	window.addEventListener('pointerover', over, { capture: true, passive: true });
	return () => {
		clearTimeout(timer);
		window.removeEventListener('pointerover', over, { capture: true });
	};
}
