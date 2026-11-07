// Picture records: the metadata server's stored version, size and main
// colour of each music picture. Every music answer carries the records of
// the pictures it shows ("pictures": { "album:<mbid>": {v, w, h, color} }),
// and api.ts keeps them here, so a card draws its box and colour before
// the picture arrives and asks the versioned address (pv), which the
// browser keeps a year.

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
