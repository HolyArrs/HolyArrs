import { to } from './paths.ts';

// TMDB pictures at the size each slot shows: posters
// w342, or w500 on phone and sharp screens; backdrops in rows w780; heroes
// w1280, or original past 1600 pixels. They load through HolyArrs' artwork
// cache (/api/v1/artwork/tmdb/<size>/<file>): the server fetches each
// picture once at TMDB's own size, with no Referer, keeps it on disk (at
// most six months, TMDB's rule) and serves it with a year-long cache
// header. Few sizes per slot means one row's poster is the next row's
// cached copy. Other pictures (HolyArrs' own artwork cache, other sites)
// are left as they are.
//
// No srcset on purpose: Chrome keeps every picture with a srcset in a
// viewport listener after its page is gone, and with it that whole page
// (measured: 20 Explore visits kept 120,000 DOM nodes and 1,700 listeners).
const tmdb = /^https:\/\/image\.tmdb\.org\/t\/p\/(?:w\d+|original)\/([A-Za-z0-9_-]+\.(?:jpg|png|webp))$/;

// Any TMDB picture address, at any size.
const tmdbAny = /^https?:\/\/image\.tmdb\.org\/t\/p\/(w\d+|h\d+|original)\/([A-Za-z0-9_-]+\.(?:jpg|png|webp))$/;
// The sizes the artwork cache fetches (internal/library/artwork.go tmdbSizes); others come at w500.
const cacheSizes = new Set(['w92', 'w154', 'w185', 'w300', 'w342', 'w500', 'w780', 'w1280', 'h632', 'original']);

/** proxied sends a TMDB picture through HolyArrs' artwork cache, so the browser never calls TMDB itself. Other pictures are left as they are. */
export function proxied(url: string | null | undefined): string {
	const m = url ? tmdbAny.exec(url) : null;
	if (!m) return url ?? '';
	return to(`/api/v1/artwork/tmdb/${cacheSizes.has(m[1]) ? m[1] : 'w500'}/${m[2]}`);
}

const width = { poster: 342, backdrop: 780, large: 1280 } as const;

// The slot sizes: a card that knows its rendered width takes the smallest
// that still covers it at the screen's pixel ratio (w185 only for small
// thumbnails, w500 for phone cards and sharp screens).
const steps = { poster: [185, 342, 500], backdrop: [780], large: [1280] } as const;

/** pick is the TMDB width for css pixels shown (0: the default size); never above the slot's largest. */
export function pick(as: 'poster' | 'backdrop' | 'large', css = 0, ratio = globalThis.devicePixelRatio || 1): number {
	if (!(css > 0)) return width[as];
	const need = css * ratio;
	return steps[as].find((w) => w >= need) ?? steps[as][steps[as].length - 1];
}

export function sized(url: string | null | undefined, as: 'poster' | 'backdrop' | 'large', css = 0): string {
	const m = url ? tmdb.exec(url) : null;
	return m ? to(`/api/v1/artwork/tmdb/w${pick(as, css)}/${m[1]}`) : (url ?? '');
}

/** heroSize is the TMDB size a hero backdrop css pixels wide needs at the
 * pixel ratio: w1280, else original past 1600 pixels (a phone hero crops a
 * wide backdrop by height, so it never gets less than w1280). */
export function heroSize(css: number, ratio = globalThis.devicePixelRatio || 1): string {
	return css * ratio <= 1600 ? 'w1280' : 'original';
}

/** sharpen moves an artwork-cache TMDB address (from sized) to heroSize; other addresses stay. */
export function sharpen(url: string, css: number, ratio = globalThis.devicePixelRatio || 1): string {
	return url.replace(/(\/api\/v1\/artwork\/tmdb\/)(?:w\d+|original)(\/[^/]+)$/, `$1${heroSize(css, ratio)}$2`);
}

const anyTmdb = /^(.*?\/(?:t\/p|api\/v1\/artwork\/tmdb))\/(?:w\d+|h\d+|original)\/([A-Za-z0-9_-]+\.(?:jpg|png|webp))$/;

/** fitTile swaps a TMDB picture's size for one that covers a tall tile css
 * pixels wide at the screen's pixel ratio: profiles (people) w185, w500 or
 * original; backdrops w780, w1280 or original. Other pictures are left as they are. */
export function fitTile(url: string, css: number, person: boolean, ratio = globalThis.devicePixelRatio || 1): string {
	const m = anyTmdb.exec(url);
	if (!m || !(css > 0)) return proxied(url);
	const need = css * ratio;
	// A tall 3:4 tile: a profile is cut by height, a backdrop fills by height (16:9, so about 2.4x the tile width).
	const size = person ? (need <= 185 ? 'w185' : need <= 500 ? 'w500' : 'original') : need * 2.4 <= 780 ? 'w780' : need * 2.4 <= 1280 ? 'w1280' : 'original';
	return proxied(`${m[1]}/${size}/${m[2]}`);
}

const warmed = new Set<string>();
/** preloadHero starts fetching a title hero's backdrop at the size the hero
 * will ask for (the page width), so it is in the cache when the hero renders. */
export function preloadHero(backdrop: string | null | undefined): void {
	if (!backdrop || typeof window === 'undefined') return;
	const url = sharpen(sized(backdrop, 'backdrop'), window.innerWidth);
	if (!url || warmed.has(url)) return;
	warmed.add(url);
	const im = new Image();
	im.decoding = 'async';
	im.fetchPriority = 'high';
	im.src = url;
}
