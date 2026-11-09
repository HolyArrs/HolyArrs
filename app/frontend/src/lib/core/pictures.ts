// Picture records and cover URL sizing for artwork.

import { to } from './paths.ts';

export type PictureRecord = { v: string; w: number; h: number; color: string };

// Picture facts keyed by kind and id, filled from server payloads.
const records = new Map<string, PictureRecord>();
// Cap so a long session cannot grow the cache without bound.
const MAX = 20000;

const route = /\/music-trending\/art\/(album|artist|wide|logo)\/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})(?=[?#]|$)/;
// Accepts only plain tokens so a bad value never lands in a URL.
const version = /^[A-Za-z0-9_-]{1,64}$/;
const colour = /^#[0-9a-f]{6}$/;

// Bumped to refetch artist photos that browsers cached before the picture records existed.
const artistPhotoVersion = '4';

// Image hosts whose URLs encode a size, so we can ask for the one we need.
const caa = /^(https?:\/\/(?:www\.)?coverartarchive\.org\/release(?:-group)?\/[^/]+\/(?:front|back|\d+))(?:-(?:250|500|1200))?(?:\.jpg)?$/;
const proxied = /\/api\/v1\/(?:music-trending\/art\/album\/[^/?]+|albums\/\d+\/cover)(?:\?.*)?$/;
const thumbOnly = /\/api\/v1\/(?:artwork\/[0-9a-f]+\.(?:jpg|jpeg|png|webp)|music-trending\/art\/artist\/[^/?]+)(?:\?(?!.*size=).*)?$/;

const deezer = /^(https?:\/\/[a-z0-9-]+\.dzcdn\.net\/images\/(?:cover|artist)\/[0-9a-f]+\/)\d+x\d+(-.*)?$/;
const itunes = /^(https?:\/\/[a-z0-9-]+\.mzstatic\.com\/image\/thumb\/.+\/)\d+x\d+(bb|cc)?(\.(?:jpg|png|webp))$/;

// Takes untrusted payload data and keeps only well-formed records.
export function registerPictures(raw: unknown): void {
	if (!raw || typeof raw !== 'object') return;
	if (records.size > MAX) records.clear();
	for (const [key, r] of Object.entries(raw as Record<string, unknown>)) {
		if (!/^(album|artist|wide|logo):[0-9a-f-]{36}$/.test(key) || !r || typeof r !== 'object') continue;
		const { v, w, h, color } = r as Record<string, unknown>;
		if (typeof v !== 'string' || !version.test(v) || typeof w !== 'number' || typeof h !== 'number' || !(w > 0) || !(h > 0)) continue;
		records.set(key, { v, w, h, color: typeof color === 'string' && colour.test(color) ? color : '' });
	}
}

// Maps an art route URL to its record key.
export function pictureKey(src: string | null | undefined): string | null {
	const m = src ? route.exec(src.split('?')[0]) : null;
	return m ? `${m[1]}:${m[2]}` : null;
}

export function pictureOf(src: string | null | undefined): PictureRecord | undefined {
	const k = pictureKey(src);
	return k ? records.get(k) : undefined;
}

export function pictureFor(kind: 'album' | 'artist' | 'wide' | 'logo', mbid: string | null | undefined): PictureRecord | undefined {
	return mbid ? records.get(`${kind}:${mbid.toLowerCase()}`) : undefined;
}

// Adds the cache-busting version so a changed picture is refetched.
export function withVersion(src: string): string {
	const r = pictureOf(src);
	if (!r || /[?&]pv=/.test(src)) return src;
	return `${src}${src.includes('?') ? '&' : '?'}pv=${r.v}`;
}

export function forgetPictures(): void {
	records.clear();
}

export const versioned = (src: string) => {
	const v = withVersion(src);
	if (v !== src || !src.includes('/music-trending/art/artist/')) return v;
	return src + (src.includes('?') ? '&' : '?') + 'v=' + artistPhotoVersion;
};

export const appPicture = (src: string | undefined | null) => (!src ? null : src.startsWith('/') ? to(versioned(src)) : src);

// Picks the smallest size that stays sharp: hero art can go larger, small tiles stay at 250.
export function coverSize(css: number, ratio = globalThis.devicePixelRatio || 1, hero = false): 250 | 500 | 1200 {
	const px = css * ratio;
	if (hero) return px > 500 ? 1200 : 500;
	return css <= 125 && px <= 250 ? 250 : 500;
}

// Rewrites a cover URL to the size it will be drawn at.
export function coverFor(src: string | null | undefined, css: number, ratio = globalThis.devicePixelRatio || 1, hero = false): { src: string | null } {
	const at = src ? coverAt(src) : null;
	if (at) return { src: at(coverSize(css, ratio, hero)) };
	if (src && !hero && thumbOnly.test(src) && coverSize(css, ratio) === 250) return { src: `${src}${src.includes('?') ? '&' : '?'}size=250` };
	return { src: src ?? null };
}

// Returns a resizer for hosts we know how to size, or null for anything else.
function coverAt(src: string): ((s: number) => string) | null {
	let m = caa.exec(src);
	if (m) return (s) => `${m![1]}-${s}`;
	if ((m = deezer.exec(src))) return (s) => `${m![1]}${s}x${s}${m![2] ?? ''}`;
	if ((m = itunes.exec(src))) return (s) => `${m![1]}${s}x${s}${m![2] || 'bb'}${m![3]}`;
	if (proxied.test(src)) return (s) => `${src}${src.includes('?') ? '&' : '?'}size=${s}`;
	return null;
}
