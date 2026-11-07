import { to } from './paths.ts';

const tmdb = /^https:\/\/image\.tmdb\.org\/t\/p\/(?:w\d+|original)\/([A-Za-z0-9_-]+\.(?:jpg|png|webp))$/;

const tmdbAny = /^https?:\/\/image\.tmdb\.org\/t\/p\/(w\d+|h\d+|original)\/([A-Za-z0-9_-]+\.(?:jpg|png|webp))$/;
const cacheSizes = new Set(['w92', 'w154', 'w185', 'w300', 'w342', 'w500', 'w780', 'w1280', 'h632', 'original']);

export function proxied(url: string | null | undefined): string {
	const m = url ? tmdbAny.exec(url) : null;
	if (!m) return url ?? '';
	return to(`/api/v1/artwork/tmdb/${cacheSizes.has(m[1]) ? m[1] : 'w500'}/${m[2]}`);
}

const width = { poster: 342, backdrop: 780, large: 1280 } as const;

const steps = { poster: [185, 342, 500], backdrop: [780], large: [1280] } as const;

export function pick(as: 'poster' | 'backdrop' | 'large', css = 0, ratio = globalThis.devicePixelRatio || 1): number {
	if (!(css > 0)) return width[as];
	const need = css * ratio;
	return steps[as].find((w) => w >= need) ?? steps[as][steps[as].length - 1];
}

export function sized(url: string | null | undefined, as: 'poster' | 'backdrop' | 'large', css = 0): string {
	const m = url ? tmdb.exec(url) : null;
	return m ? to(`/api/v1/artwork/tmdb/w${pick(as, css)}/${m[1]}`) : (url ?? '');
}

export function heroSize(css: number, ratio = globalThis.devicePixelRatio || 1): string {
	return css * ratio <= 1600 ? 'w1280' : 'original';
}

export function sharpen(url: string, css: number, ratio = globalThis.devicePixelRatio || 1): string {
	return url.replace(/(\/api\/v1\/artwork\/tmdb\/)(?:w\d+|original)(\/[^/]+)$/, `$1${heroSize(css, ratio)}$2`);
}

const anyTmdb = /^(.*?\/(?:t\/p|api\/v1\/artwork\/tmdb))\/(?:w\d+|h\d+|original)\/([A-Za-z0-9_-]+\.(?:jpg|png|webp))$/;

export function fitTile(url: string, css: number, person: boolean, ratio = globalThis.devicePixelRatio || 1): string {
	const m = anyTmdb.exec(url);
	if (!m || !(css > 0)) return proxied(url);
	const need = css * ratio;
	const size = person ? (need <= 185 ? 'w185' : need <= 500 ? 'w500' : 'original') : need * 2.4 <= 780 ? 'w780' : need * 2.4 <= 1280 ? 'w1280' : 'original';
	return proxied(`${m[1]}/${size}/${m[2]}`);
}

const warmed = new Set<string>();
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
