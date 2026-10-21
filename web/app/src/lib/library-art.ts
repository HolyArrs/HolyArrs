// Library art sizes: how wide the banner column is for each density, how
// big posters are, and which picture size keeps each sharp at the screen's
// pixel ratio. Densities and poster sizes are remembered per member.

export type Density = 'large' | 'comfortable' | 'compact';
export type PosterSize = 's' | 'm' | 'l';

/** The banner's own shape: fanart.tv banners are 1000x185. */
export const BANNER_RATIO = 1000 / 185;

const bands: Record<Density, { share: number; min: number; max: number }> = {
	large: { share: 0.3, min: 420, max: 640 },
	comfortable: { share: 0.24, min: 300, max: 440 },
	compact: { share: 0.18, min: 200, max: 300 }
};

export const densityLabels: Record<Density, string> = { large: 'Large art', comfortable: 'Comfortable', compact: 'Compact' };
export const posterLabels: Record<PosterSize, string> = { s: 'Small posters', m: 'Medium posters', l: 'Large posters' };
/** Smallest poster column width per size, in css pixels. */
export const posterMin: Record<PosterSize, number> = { s: 132, m: 172, l: 232 };

/** bannerWidth is the banner column's css width for a table this wide. */
export function bannerWidth(tableWidth: number, density: Density): number {
	const b = bands[density] ?? bands.large;
	return Math.round(Math.max(b.min, Math.min(b.max, tableWidth * b.share)));
}

/** bannerHeight is the row height the banner's ratio gives. */
export function bannerHeight(width: number): number {
	return Math.round(width / BANNER_RATIO);
}

const anyTmdb = /^(.*?\/(?:t\/p|api\/v1\/artwork\/tmdb))\/(?:w\d+|h\d+|original)\/([A-Za-z0-9_-]+\.(?:jpg|png|webp))$/;

/** tmdbFor picks the TMDB size that covers css pixels at the ratio: w780, w1280 or original. */
export function tmdbFor(css: number, ratio = 1): 'w780' | 'w1280' | 'original' {
	const need = css * ratio;
	return need <= 780 ? 'w780' : need <= 1280 ? 'w1280' : 'original';
}

/**
 * bannerSrc sizes a banner picture for a cell css pixels wide. A backdrop
 * shown as a banner fills by width (the crop only trims top and bottom), so
 * it needs css x ratio pixels across. Fanart previews become the full file.
 * Other pictures are left as they are.
 */
export function bannerSrc(url: string, css: number, ratio = globalThis.devicePixelRatio || 1): string {
	if (!url) return url;
	if (url.includes('assets.fanart.tv/preview/')) return url.replace('assets.fanart.tv/preview/', 'assets.fanart.tv/fanart/');
	const m = anyTmdb.exec(url);
	if (!m || !(css > 0)) return url;
	return `${m[1]}/${tmdbFor(css, ratio)}/${m[2]}`;
}

/**
 * posterSrc sizes a TMDB poster for a grid column at least css pixels wide
 * (columns can stretch up to about 1.6x their minimum before another fits).
 */
export function posterSrc(url: string, css: number, ratio = globalThis.devicePixelRatio || 1): string {
	const m = url ? anyTmdb.exec(url) : null;
	if (!m || !(css > 0)) return url;
	const need = css * 1.6 * ratio;
	const size = need <= 342 ? 'w342' : need <= 500 ? 'w500' : need <= 780 ? 'w780' : 'original';
	return `${m[1]}/${size}/${m[2]}`;
}

type Store = Pick<Storage, 'getItem' | 'setItem'> | undefined;
const key = (what: string, member: number | undefined) => `holyarrs.library-${what}.${member ?? 'me'}`;

export function loadDensity(member: number | undefined, store: Store = globalThis.localStorage): Density {
	try {
		const v = store?.getItem(key('density', member));
		return v === 'comfortable' || v === 'compact' ? v : 'large';
	} catch {
		return 'large';
	}
}
export function saveDensity(member: number | undefined, d: Density, store: Store = globalThis.localStorage) {
	try {
		store?.setItem(key('density', member), d);
	} catch { /* Private windows may refuse. */ }
}
export function loadPosterSize(member: number | undefined, store: Store = globalThis.localStorage): PosterSize {
	try {
		const v = store?.getItem(key('poster-size', member));
		return v === 's' || v === 'l' ? v : 'm';
	} catch {
		return 'm';
	}
}
export function savePosterSize(member: number | undefined, s: PosterSize, store: Store = globalThis.localStorage) {
	try {
		store?.setItem(key('poster-size', member), s);
	} catch { /* Private windows may refuse. */ }
}
