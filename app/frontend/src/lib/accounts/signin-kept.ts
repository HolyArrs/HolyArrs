import { THUMB_W, sized, type ArtPick, type ArtShot } from './signin-art.ts';
import { to } from '#lib/core/paths.ts';

export type Thumb = { url: string; t24: string; t64: string; pos: string };

const CACHE = 'holyarrs-signin-art';
const SEED = 'holyarrs-signin-seed'; // The style element app.html adds for the first frame.

export function screen() {
	return { vw: window.innerWidth, vh: window.innerHeight, dpr: window.devicePixelRatio || 1 };
}

export async function ready(url: string): Promise<boolean> {
	// CORS mode: the same cache entry as the reveal's WebGL copy. The pictures are same-origin, so this never needs a CORS header; should it fail anyway, a plain load still shows the photo (no reveal).
	for (const cors of [true, false]) {
		const img = new Image();
		if (cors) img.crossOrigin = 'anonymous';
		img.decoding = 'async';
		img.src = url;
		try {
			await img.decode();
			return true;
		} catch {
			/* Try the plain load. */
		}
	}
	return false;
}

/** Same-origin pictures only (lists kept from before held TMDB addresses), under the app's base path. */
export const local = (u: unknown) => typeof u === 'string' && u.startsWith('/') && !u.startsWith('//');

export function ownShots(list: unknown, prefix: boolean): ArtShot[] {
	if (!Array.isArray(list)) return [];
	const at = (u: string) => (prefix ? to(u) : u);
	return list
		.filter((x: ArtShot) => local(x?.backdrop?.url))
		.map((x: ArtShot) => ({
			backdrop: { ...x.backdrop, url: at(x.backdrop.url) },
			poster: x.poster && local(x.poster.url) ? { ...x.poster, url: at(x.poster.url) } : undefined,
			logo: local(x.logo) ? at(x.logo) : ''
		}));
}

export function readKept(): { shots: ArtShot[]; first?: string; thumb: Thumb | null } {
	try {
		const v = JSON.parse(localStorage.getItem(CACHE) ?? 'null');
		const t = v?.thumb;
		const ok = t && typeof t.url === 'string' && /^data:image\/jpeg;base64,/.test(t.t64 ?? '');
		return { shots: ownShots(v?.shots, false), first: v?.first, thumb: ok && local(t.url) ? t : null };
	} catch {
		return { shots: [], thumb: null };
	}
}

export function writeCache(shots: ArtShot[], first: string, thumb: Thumb | null) {
	try {
		localStorage.setItem(CACHE, JSON.stringify({ shots, first, thumb: thumb?.url === first ? thumb : null }));
	} catch {
		/* Private window or full storage: the next visit fetches again. */
	}
}

// A tiny copy of the next visit's first picture: 24 and 64 px wide JPEGs (about 2 KB together). The picture is same-origin, so the canvas can be read back.
export async function makeThumb(pick: ArtPick): Promise<Thumb | null> {
	const img = new Image();
	img.crossOrigin = 'anonymous';
	img.src = sized(pick.url, pick.kind === 'poster' ? 342 : 300);
	try {
		await img.decode();
		const at = (w: number) => {
			const c = document.createElement('canvas');
			c.width = w;
			c.height = Math.max(1, Math.round((w * img.naturalHeight) / img.naturalWidth));
			const g = c.getContext('2d');
			if (!g) throw new Error('no 2d context');
			g.imageSmoothingQuality = 'high';
			g.drawImage(img, 0, 0, c.width, c.height);
			return c.toDataURL('image/jpeg', 0.72);
		};
		return { url: pick.url, t24: at(24), t64: at(THUMB_W), pos: pick.position };
	} catch {
		return null;
	}
}

export function unseed() {
	document.getElementById(SEED)?.remove();
	document.documentElement.classList.remove('sa-seeded');
}
