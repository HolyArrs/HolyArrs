// Rewrites TMDB picture addresses to the cached, right-sized copy served by the app.
import { to } from './paths.ts';
import { minimalLook } from './look.svelte.ts';

// Strict form: https only, used when we pick the size ourselves.
const tmdb = /^https:\/\/image\.tmdb\.org\/t\/p\/(?:w\d+|original)\/([A-Za-z0-9_-]+\.(?:jpg|png|webp))$/;

// Loose form: also http and any size, so old links still get proxied.
const tmdbAny = /^https?:\/\/image\.tmdb\.org\/t\/p\/(w\d+|h\d+|original)\/([A-Za-z0-9_-]+\.(?:jpg|png|webp))$/;
// Sizes the server cache holds; anything else falls back to w500.
const cacheSizes = new Set(['w92', 'w154', 'w185', 'w300', 'w342', 'w500', 'w780', 'w1280', 'h632', 'original']);

// Default width per use when the real size is not known.
const width = { poster: 342, backdrop: 780, large: 1280 } as const;

// Sizes offered per use, smallest first.
const steps = { poster: [185, 342, 500], backdrop: [780], large: [1280] } as const;
// Matches both direct and already proxied addresses.
const anyTmdb = /^(.*?\/(?:t\/p|api\/v1\/artwork\/tmdb))\/(?:w\d+|h\d+|original)\/([A-Za-z0-9_-]+\.(?:jpg|png|webp))$/;
// Heroes already requested, so each is fetched once.
const warmed = new Set<string>();

// Sends a TMDB picture through our cache so the browser never calls TMDB.
export function proxied(url: string | null | undefined): string {
	const m = url ? tmdbAny.exec(url) : null;
	if (!m) return url ?? '';
	return to(`/api/v1/artwork/tmdb/${cacheSizes.has(m[1]) ? m[1] : 'w500'}/${m[2]}`);
}

// Smallest step that stays sharp at this width and screen density.
export function pick(as: 'poster' | 'backdrop' | 'large', css = 0, ratio = globalThis.devicePixelRatio || 1): number {
	if (!(css > 0)) return width[as];
	const need = css * ratio;
	return steps[as].find((w) => w >= need) ?? steps[as][steps[as].length - 1];
}

export function sized(url: string | null | undefined, as: 'poster' | 'backdrop' | 'large', css = 0): string {
	const m = url ? tmdb.exec(url) : null;
	return m ? to(`/api/v1/artwork/tmdb/w${pick(as, css)}/${m[1]}`) : (url ?? '');
}

// Hero art only goes to original on wide or dense screens, where w1280 would look soft.
export function heroSize(css: number, ratio = globalThis.devicePixelRatio || 1): string {
	return css * ratio <= 1600 ? 'w1280' : 'original';
}

export function sharpen(url: string, css: number, ratio = globalThis.devicePixelRatio || 1): string {
	return url.replace(/(\/api\/v1\/artwork\/tmdb\/)(?:w\d+|original)(\/[^/]+)$/, `$1${heroSize(css, ratio)}$2`);
}

// Backdrops are cropped into tiles, so they need a bigger source than the tile width (2.4x).
export function fitTile(url: string, css: number, person: boolean, ratio = globalThis.devicePixelRatio || 1): string {
	const m = anyTmdb.exec(url);
	if (!m || !(css > 0)) return proxied(url);
	const need = css * ratio;
	const size = person ? (need <= 185 ? 'w185' : need <= 500 ? 'w500' : 'original') : need * 2.4 <= 780 ? 'w780' : need * 2.4 <= 1280 ? 'w1280' : 'original';
	return proxied(`${m[1]}/${size}/${m[2]}`);
}

// Starts the hero fetch early at high priority so it is cached when the page paints.
export function preloadHero(backdrop: string | null | undefined): void {
	if (!backdrop || typeof window === 'undefined' || minimalLook()) return;
	const url = sharpen(sized(backdrop, 'backdrop'), window.innerWidth);
	if (!url || warmed.has(url)) return;
	warmed.add(url);
	const im = new Image();
	im.decoding = 'async';
	im.fetchPriority = 'high';
	im.src = url;
}
