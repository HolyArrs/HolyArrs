import { proxied } from './tmdb-image.ts';

// Library art sizes: how wide the banner column is for each density, how
// big posters are, and which picture size keeps each sharp at the screen's
// pixel ratio. Densities and poster sizes are remembered per member.

export type Density = 'large' | 'comfortable' | 'compact';
export type PosterSize = 's' | 'm' | 'l';

/** The banner's own shape: title banners are 1000x185. */
export const BANNER_RATIO = 1000 / 185;
/** Music's banner is taller (1000x400): artist art is a square photo, and across the Library's artists this shape (with a 35% crop) keeps a face and shoulders in view where 1000x320 cut chins and heads. */
export const MUSIC_BANNER_RATIO = 1000 / 400;

/** The kinds the Library shows art for; Music is 'artist'. */
export type ArtKind = 'movie' | 'show' | 'artist' | string;

/** bannerRatio is the banner's width over its height for a kind of title. */
export function bannerRatio(kind?: ArtKind): number {
	return kind === 'artist' ? MUSIC_BANNER_RATIO : BANNER_RATIO;
}

/** The banner's shape for a kind as a css aspect-ratio (the --banner-ratio custom property). */
export function bannerAspect(kind?: ArtKind): string {
	return kind === 'artist' ? '1000 / 400' : '1000 / 185';
}

/** The poster's shape for a kind as a css aspect-ratio: square for Music, 2:3 for the rest. */
export function posterRatio(kind?: ArtKind): string {
	return kind === 'artist' ? '1 / 1' : '2 / 3';
}

const bands: Record<Density, { share: number; min: number; max: number }> = {
	large: { share: 0.3, min: 420, max: 640 },
	comfortable: { share: 0.24, min: 300, max: 440 },
	compact: { share: 0.18, min: 200, max: 300 }
};

export const densityLabels: Record<Density, string> = { large: 'Large', comfortable: 'Comfortable', compact: 'Compact' };
export const posterLabels: Record<PosterSize, string> = { s: 'Small', m: 'Medium', l: 'Large' };
/** Smallest poster column width per size, in css pixels. */
export const posterMin: Record<PosterSize, number> = { s: 132, m: 172, l: 232 };
/** Music's square art is a third shorter than a poster, so its columns pack a little tighter, like Apple Music's artist grid (six across at 1440). */
export const squareMin: Record<PosterSize, number> = { s: 124, m: 160, l: 212 };

/** Music's smallest column on phones: two squares across at medium, so names stay readable. */
export const squarePhoneMin: Record<PosterSize, number> = { s: 100, m: 136, l: 156 };

/** posterMinFor is the smallest poster column for a kind and size. */
export function posterMinFor(size: PosterSize, kind?: ArtKind): number {
	return (kind === 'artist' ? squareMin : posterMin)[size] ?? posterMin.m;
}

/** bannerWidth is the banner column's css width for a table this wide. */
export function bannerWidth(tableWidth: number, density: Density): number {
	const b = bands[density] ?? bands.large;
	return Math.round(Math.max(b.min, Math.min(b.max, tableWidth * b.share)));
}

/** bannerHeight is the row height the kind's banner shape gives. */
export function bannerHeight(width: number, kind?: ArtKind): number {
	return Math.round(width / bannerRatio(kind));
}

const anyTmdb = /^(.*?\/(?:t\/p|api\/v1\/artwork\/tmdb))\/(?:w\d+|h\d+|original)\/([A-Za-z0-9_-]+\.(?:jpg|png|webp))$/;

/** tmdbFor picks the TMDB size that covers css pixels at the ratio: w780, w1280 or original. */
export function tmdbFor(css: number, ratio = 1): 'w780' | 'w1280' | 'original' {
	const need = css * ratio;
	return need <= 780 ? 'w780' : need <= 1280 ? 'w1280' : 'original';
}

/**
 * bannerSrc sizes a TMDB backdrop shown as a banner for a cell css pixels
 * wide: it fills by width (the crop only trims top and bottom), so it needs
 * css x ratio pixels across. Other pictures are left as they are.
 */
export function bannerSrc(url: string, css: number, ratio = globalThis.devicePixelRatio || 1): string {
	if (!url) return url;
	const m = anyTmdb.exec(url);
	if (!m || !(css > 0)) return proxied(url);
	return proxied(`${m[1]}/${tmdbFor(css, ratio)}/${m[2]}`);
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

/** The sizes TMDB keeps for each kind of picture (largest last; 'original' is past the end). */
export const TMDB_SIZES = {
	poster: [92, 154, 185, 342, 500, 780],
	profile: [45, 185, 632],
	backdrop: [300, 780, 1280],
	logo: [92, 154, 185, 300, 500]
} as const;
export type TmdbKind = keyof typeof TMDB_SIZES;

/**
 * tmdbSized points a TMDB picture at the smallest size of its kind that covers
 * css x ratio pixels across (profile's top size is a height, h632). Other
 * pictures are left as they are.
 */
export function tmdbSized(url: string | null | undefined, kind: TmdbKind, css: number, ratio = globalThis.devicePixelRatio || 1): string {
	const m = url ? anyTmdb.exec(url) : null;
	if (!m || !(css > 0)) return proxied(url);
	const sizes = TMDB_SIZES[kind];
	const need = css * ratio;
	const w = sizes.find((s) => s >= need) ?? sizes[sizes.length - 1];
	return proxied(`${m[1]}/${kind === 'profile' && w === 632 ? 'h632' : `w${w}`}/${m[2]}`);
}
