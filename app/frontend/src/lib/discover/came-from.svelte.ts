// Where the person came from, per history entry, so a title page's first breadcrumb can say "Search" or "Explore" and step back there (with its scroll and state) instead of always pointing at the Library. Kept per tab in sessionStorage.

import { t } from '#lib/core/i18n/i18n.svelte.ts';
export type CameFrom = { path: string; label: string; y?: number };

const KEY = 'holyarrs:came-from';
let before: CameFrom | null = null;

/** SvelteKit's index for the current history entry, or null. */
export function historyIndex(): string | null {
	const s = history.state as Record<string, unknown> | null;
	// Older SvelteKit keeps the index at 'sveltekit:history'; newer under 'sveltekit:metadata'.historyIndex.
	const meta = s?.['sveltekit:metadata'] as Record<string, unknown> | undefined;
	const i = s?.['sveltekit:history'] ?? meta?.historyIndex;
	return typeof i === 'number' ? String(i) : null;
}

/** A page's name from its title ("Search · HolyArrs" becomes "Search"). */
export function pageLabel(title: string): string {
	return title.split(' · ')[0].trim();
}

/** True for sign-in, welcome and other auth pages, which are never a place to go back to. */
export function isAuthPath(path: string): boolean {
	return /^\/(login|welcome|logout|signin|sign-in|signup|setup|auth)(\/|$|\?)/.test(path);
}

/** Notes the page being left; the layout calls it before each navigation. */
export function leaving() {
	if (typeof document === 'undefined') return;
	if (isAuthPath(location.pathname)) {
		before = null;
		return;
	}
	before = { path: location.pathname + location.search, label: pageLabel(document.title), y: typeof window === 'undefined' ? 0 : Math.round(window.scrollY) };
}

/** Records where this new entry came from; going back and forth reads it again. */
export function arrived(type: string) {
	const i = historyIndex();
	if (i === null || !before || type === 'popstate' || type === 'enter') return;
	const all = read();
	all[i] = before;
	try {
		sessionStorage.setItem(KEY, JSON.stringify(all));
	} catch {
		/* Private mode: the breadcrumb keeps its default. */
	}
}

/** The page this entry was opened from, or null (a fresh visit or a reload of the first page). */
export function cameFrom(): CameFrom | null {
	if (typeof history === 'undefined') return null;
	const i = historyIndex();
	const found = i === null ? null : (read()[i] ?? null);
	// A sign-in page or a not-found page (such as an old /title/tv/ link that moved on) is no place to step back to.
	return found && (isAuthPath(found.path) || found.label === 'Not found' || found.label === t('discover.titlePage.notFound')) ? null : found;
}

function read(): Record<string, CameFrom> {
	try {
		return JSON.parse(sessionStorage.getItem(KEY) ?? '{}');
	} catch {
		return {};
	}
}
