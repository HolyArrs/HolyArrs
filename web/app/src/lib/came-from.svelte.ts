// Where the person came from, per history entry, so a title page's first
// breadcrumb can say "Search" or "Explore" and step back there (with its
// scroll and state) instead of always pointing at the Library.
// Kept per tab in sessionStorage, keyed by SvelteKit's history index.

export type CameFrom = { path: string; label: string };

const KEY = 'holyarrs:came-from';
let before: CameFrom | null = null;

function read(): Record<string, CameFrom> {
	try {
		return JSON.parse(sessionStorage.getItem(KEY) ?? '{}');
	} catch {
		return {};
	}
}

function index(): string | null {
	const s = history.state as Record<string, unknown> | null;
	const i = s?.['sveltekit:history'];
	return typeof i === 'number' ? String(i) : null;
}

/** pageLabel is a page's name from its title ("Search · HolyArrs" → "Search"). */
export function pageLabel(title: string): string {
	return title.split(' · ')[0].trim();
}

/** leaving notes the page being left (the layout calls it before each navigation). */
export function leaving() {
	if (typeof document === 'undefined') return;
	before = { path: location.pathname + location.search, label: pageLabel(document.title) };
}

/** arrived records where this new entry came from; going back and forth reads it again. */
export function arrived(type: string) {
	const i = index();
	if (i === null || !before || type === 'popstate' || type === 'enter') return;
	const all = read();
	all[i] = before;
	try {
		sessionStorage.setItem(KEY, JSON.stringify(all));
	} catch {
		/* private mode: the breadcrumb keeps its default */
	}
}

/** cameFrom is the page this entry was opened from, or null (a fresh visit or a reload of the first page). */
export function cameFrom(): CameFrom | null {
	if (typeof history === 'undefined') return null;
	const i = index();
	return i === null ? null : (read()[i] ?? null);
}

/** fromIcon picks the breadcrumb icon for a page path. */
export function fromIcon(path: string): 'search' | 'compass' | 'people' | 'home' | null {
	if (path.includes('/search')) return 'search';
	if (path.includes('/explore')) return 'compass';
	if (path.includes('/people')) return 'people';
	const bare = path.split('?')[0];
	if (bare === '/' || bare.endsWith('/home')) return 'home';
	return null;
}
