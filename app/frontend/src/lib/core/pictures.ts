// Picture records: the metadata server's stored version, size and main
// colour of each music picture. Every music answer carries the records of
// the pictures it shows ("pictures": { "album:<mbid>": {v, w, h, color} }),
// and api.ts keeps them here, so a card draws its box and colour before
// the picture arrives and asks the versioned address (pv), which the
// browser keeps a year.

import { to } from './paths.ts';

export type PictureRecord = { v: string; w: number; h: number; color: string };

const records = new Map<string, PictureRecord>();
const MAX = 20000;

const route = /\/music-trending\/art\/(album|artist|wide|logo)\/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})(?=[?#]|$)/;
const version = /^[A-Za-z0-9_-]{1,64}$/;
const colour = /^#[0-9a-f]{6}$/;

/** registerPictures keeps an answer's records; anything odd is skipped. */
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

/** pictureKey is the record key of a music picture address ("album:<mbid>"), or null. */
export function pictureKey(src: string | null | undefined): string | null {
	const m = src ? route.exec(src.split('?')[0]) : null;
	return m ? `${m[1]}:${m[2]}` : null;
}

/** pictureOf is the record of a music picture address, when an answer carried it. */
export function pictureOf(src: string | null | undefined): PictureRecord | undefined {
	const k = pictureKey(src);
	return k ? records.get(k) : undefined;
}

/** pictureFor is the record of one kind of picture of an item. */
export function pictureFor(kind: 'album' | 'artist' | 'wide' | 'logo', mbid: string | null | undefined): PictureRecord | undefined {
	return mbid ? records.get(`${kind}:${mbid.toLowerCase()}`) : undefined;
}

/** withVersion adds the stored version (pv) to a music picture address that has none. */
export function withVersion(src: string): string {
	const r = pictureOf(src);
	if (!r || /[?&]pv=/.test(src)) return src;
	return `${src}${src.includes('?') ? '&' : '?'}pv=${r.v}`;
}

/** ratioOf is a picture's stored width over height (logos, wide backgrounds), or undefined. */
export function ratioOf(r: PictureRecord | undefined): number | undefined {
	return r && r.w > 0 && r.h > 0 ? r.w / r.h : undefined;
}

/** forgetPictures empties the records (tests). */
export function forgetPictures(): void {
	records.clear();
}

// Bumped when artist photos change source, so browsers drop copies they were told to keep.
const artistPhotoVersion = '4';

/** versioned adds a music picture's stored version (pv, kept a year) when an
 * answer carried its record; an artist photo without one keeps the old
 * cache-busting mark. */
export const versioned = (src: string) => {
	const v = withVersion(src);
	if (v !== src || !src.includes('/music-trending/art/artist/')) return v;
	return src + (src.includes('?') ? '&' : '?') + 'v=' + artistPhotoVersion;
};

/** appPicture puts a server picture path under the app's base path; outside URLs stay. */
export const appPicture = (src: string | undefined | null) => (!src ? null : src.startsWith('/') ? to(versioned(src)) : src);

const caa = /^(https?:\/\/(?:www\.)?coverartarchive\.org\/release(?:-group)?\/[^/]+\/(?:front|back|\d+))(?:-(?:250|500|1200))?(?:\.jpg)?$/;
const proxied = /\/api\/v1\/(?:music-trending\/art\/album\/[^/?]+|albums\/\d+\/cover)(?:\?.*)?$/;
// Library pictures and artist photos keep a 250 px thumbnail copy only (Poster's rule).
const thumbOnly = /\/api\/v1\/(?:artwork\/[0-9a-f]+\.(?:jpg|jpeg|png|webp)|music-trending\/art\/artist\/[^/?]+)(?:\?(?!.*size=).*)?$/;

const deezer = /^(https?:\/\/[a-z0-9-]+\.dzcdn\.net\/images\/(?:cover|artist)\/[0-9a-f]+\/)\d+x\d+(-.*)?$/;
const itunes = /^(https?:\/\/[a-z0-9-]+\.mzstatic\.com\/image\/thumb\/.+\/)\d+x\d+(bb|cc)?(\.(?:jpg|png|webp))$/;

/** coverAt points a sized cover link (our cover routes, or a chart's own
 * Cover Art Archive, Deezer or Apple link) at s px, or null when the link
 * has no size form. */
function coverAt(src: string): ((s: number) => string) | null {
	let m = caa.exec(src);
	if (m) return (s) => `${m![1]}-${s}`;
	if ((m = deezer.exec(src))) return (s) => `${m![1]}${s}x${s}${m![2] ?? ''}`;
	if ((m = itunes.exec(src))) return (s) => `${m![1]}${s}x${s}${m![2] || 'bb'}${m![3]}`;
	// Our cover proxies take ?size= (the server picks that size and caches each).
	if (proxied.test(src)) return (s) => `${src}${src.includes('?') ? '&' : '?'}size=${s}`;
	return null;
}

/** coverSize is the slot size of a cover css pixels wide (one size per slot): 500 for every card, 250 only for small thumbnails (up
 * to 125 css pixels, on screens up to 2x), 1200 only for a hero on a sharp
 * screen. */
export function coverSize(css: number, ratio = globalThis.devicePixelRatio || 1, hero = false): 250 | 500 | 1200 {
	const px = css * ratio;
	if (hero) return px > 500 ? 1200 : 500;
	return css <= 125 && px <= 250 ? 250 : 500;
}

/** coverFor sizes a cover for its slot (coverSize). No srcset: one address
 * per slot, so a row's cover is the next page's cached copy. Other pictures
 * stay as they are. */
export function coverFor(src: string | null | undefined, css: number, ratio = globalThis.devicePixelRatio || 1, hero = false): { src: string | null } {
	const at = src ? coverAt(src) : null;
	if (at) return { src: at(coverSize(css, ratio, hero)) };
	if (src && !hero && thumbOnly.test(src) && coverSize(css, ratio) === 250) return { src: `${src}${src.includes('?') ? '&' : '?'}size=250` };
	return { src: src ?? null };
}
