import { to } from './paths.ts';

// TMDB pictures in the size a row or hero shows: w342 posters and w780
// backdrops, loaded through HolyArrs' artwork cache
// (/api/v1/artwork/tmdb/<size>/<file>): the server fetches each picture
// once at TMDB's own size, keeps it on disk and serves it with a year-long
// cache header. Full-size pictures in long rows are what made Explore heavy.
// Other pictures (HolyArrs' own artwork cache, other sites) are left as
// they are.
//
// No srcset on purpose: Chrome keeps every picture with a srcset in a
// viewport listener after its page is gone, and with it that whole page
// (measured: 20 Explore visits kept 120,000 DOM nodes and 1,700 listeners).
const tmdb = /^https:\/\/image\.tmdb\.org\/t\/p\/(?:w\d+|original)\/([A-Za-z0-9_-]+\.(?:jpg|png|webp))$/;

const width = { poster: 342, backdrop: 780, large: 1280 } as const;

// TMDB's own sizes up to each default: a card that knows its rendered width
// takes the smallest one that still covers it at the screen's pixel ratio.
const steps = { poster: [92, 154, 185, 342], backdrop: [300, 780], large: [1280] } as const;

/** pick is the TMDB width for css pixels shown (0: the default size); never above the default. */
export function pick(as: 'poster' | 'backdrop' | 'large', css = 0, ratio = globalThis.devicePixelRatio || 1): number {
	if (!(css > 0)) return width[as];
	const need = css * ratio;
	return steps[as].find((w) => w >= need) ?? width[as];
}

export function sized(url: string | null | undefined, as: 'poster' | 'backdrop' | 'large', css = 0): string {
	const m = url ? tmdb.exec(url) : null;
	return m ? to(`/api/v1/artwork/tmdb/w${pick(as, css)}/${m[1]}`) : (url ?? '');
}

/** heroSize is the TMDB size a hero backdrop css pixels wide needs at the
 * pixel ratio: w780, w1280, else original past 1600 pixels. */
export function heroSize(css: number, ratio = globalThis.devicePixelRatio || 1): string {
	const need = css * ratio;
	return need <= 780 ? 'w780' : need <= 1600 ? 'w1280' : 'original';
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
	if (!m || !(css > 0)) return url;
	const need = css * ratio;
	// A tall 3:4 tile: a profile is cut by height, a backdrop fills by height (16:9, so about 2.4x the tile width).
	const size = person ? (need <= 185 ? 'w185' : need <= 500 ? 'w500' : 'original') : need * 2.4 <= 780 ? 'w780' : need * 2.4 <= 1280 ? 'w1280' : 'original';
	return `${m[1]}/${size}/${m[2]}`;
}
