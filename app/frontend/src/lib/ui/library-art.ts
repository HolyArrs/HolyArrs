import { proxied } from '../core/tmdb-image.ts';


export type Density = 'large' | 'comfortable' | 'compact';
export type PosterSize = 's' | 'm' | 'l';

export const BANNER_RATIO = 1000 / 185;
export const MUSIC_BANNER_RATIO = 1000 / 400;

export type ArtKind = 'movie' | 'show' | 'artist' | string;

export function bannerRatio(kind?: ArtKind): number {
	return kind === 'artist' ? MUSIC_BANNER_RATIO : BANNER_RATIO;
}

export function bannerAspect(kind?: ArtKind): string {
	return kind === 'artist' ? '1000 / 400' : '1000 / 185';
}

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
export const posterMin: Record<PosterSize, number> = { s: 132, m: 172, l: 232 };
export const squareMin: Record<PosterSize, number> = { s: 124, m: 160, l: 212 };

export const squarePhoneMin: Record<PosterSize, number> = { s: 100, m: 136, l: 156 };

export function posterMinFor(size: PosterSize, kind?: ArtKind): number {
	return (kind === 'artist' ? squareMin : posterMin)[size] ?? posterMin.m;
}

export function bannerWidth(tableWidth: number, density: Density): number {
	const b = bands[density] ?? bands.large;
	return Math.round(Math.max(b.min, Math.min(b.max, tableWidth * b.share)));
}

export function bannerHeight(width: number, kind?: ArtKind): number {
	return Math.round(width / bannerRatio(kind));
}

const anyTmdb = /^(.*?\/(?:t\/p|api\/v1\/artwork\/tmdb))\/(?:w\d+|h\d+|original)\/([A-Za-z0-9_-]+\.(?:jpg|png|webp))$/;

export function tmdbFor(css: number, ratio = 1): 'w780' | 'w1280' | 'original' {
	const need = css * ratio;
	return need <= 780 ? 'w780' : need <= 1280 ? 'w1280' : 'original';
}

export function bannerSrc(url: string, css: number, ratio = globalThis.devicePixelRatio || 1): string {
	if (!url) return url;
	const m = anyTmdb.exec(url);
	if (!m || !(css > 0)) return proxied(url);
	return proxied(`${m[1]}/${tmdbFor(css, ratio)}/${m[2]}`);
}

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
	} catch {}
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
	} catch {}
}

export const TMDB_SIZES = {
	poster: [92, 154, 185, 342, 500, 780],
	profile: [45, 185, 632],
	backdrop: [300, 780, 1280],
	logo: [92, 154, 185, 300, 500]
} as const;
export type TmdbKind = keyof typeof TMDB_SIZES;

export function tmdbSized(url: string | null | undefined, kind: TmdbKind, css: number, ratio = globalThis.devicePixelRatio || 1): string {
	const m = url ? anyTmdb.exec(url) : null;
	if (!m || !(css > 0)) return proxied(url);
	const sizes = TMDB_SIZES[kind];
	const need = css * ratio;
	const w = sizes.find((s) => s >= need) ?? sizes[sizes.length - 1];
	return proxied(`${m[1]}/${kind === 'profile' && w === 632 ? 'h632' : `w${w}`}/${m[2]}`);
}
