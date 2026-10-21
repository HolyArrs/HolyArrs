// Sign-in art: which picture, at which size, for this screen. The server
// answers original picture addresses with their native size; this file picks
// the one that stays sharp on the screen it is shown on.

export type ArtImage = { url: string; w: number; h: number };
export type ArtShot = { backdrop: ArtImage; poster?: ArtImage; logo: string };
export type Screen = { vw: number; vh: number; dpr: number };

export type ArtPick = {
	/** The address to load (a TMDB size or the original). */
	url: string;
	/** Native width of the chosen size, in pixels. */
	width: number;
	/** Pixels the picture covers on this screen (width), at this DPR. */
	need: number;
	kind: 'backdrop' | 'poster';
	/** object-position: keep faces and the top of the frame. */
	position: string;
};

/** Screens narrower than this (width / height) are tall: phones in portrait. */
export const TALL = 0.8;
/** On a wide screen, a picture at least this wide counts as sharp enough
 * when nothing reaches the full need (ultrawide 5120 px screens). */
export const ULTRAWIDE_FLOOR = 3840;

const SIZES: Record<ArtPick['kind'], number[]> = { backdrop: [780, 1280], poster: [500, 780] };

/** Device pixels a cover-fit picture of w x h spans across, on the screen. */
export function coverNeed(img: { w: number; h: number }, s: Screen): number {
	const cssWidth = Math.max(s.vw, (s.vh * img.w) / img.h);
	return Math.ceil(cssWidth * Math.max(1, s.dpr));
}

/** A TMDB address at another size: /t/p/original/x.jpg -> /t/p/w1280/x.jpg. */
export function sized(url: string, size: number | 'original'): string {
	return url.replace(/\/t\/p\/[^/]+\//, `/t/p/${size === 'original' ? 'original' : 'w' + size}/`);
}

function pickSize(img: ArtImage, kind: ArtPick['kind'], need: number, s: Screen): { url: string; width: number } {
	// Big or dense screens always get the original.
	if (s.vw > 1920 || s.dpr > 1) return { url: sized(img.url, 'original'), width: img.w };
	for (const w of SIZES[kind]) if (w >= need && w <= img.w) return { url: sized(img.url, w), width: w };
	return { url: sized(img.url, 'original'), width: img.w };
}

/**
 * The picture for this screen, or null when this title has none sharp
 * enough (the caller moves on to the next title rather than stretching).
 * floor relaxes the need for very wide screens: the native width must reach
 * min(need, floor). Pass 0 for the strict gate.
 */
export function pickArt(shot: ArtShot, s: Screen, floor = 0): ArtPick | null {
	const tall = s.vw / s.vh < TALL;
	const order: ['backdrop' | 'poster', ArtImage | undefined][] = tall
		? [['poster', shot.poster], ['backdrop', shot.backdrop]]
		: [['backdrop', shot.backdrop]];
	for (const [kind, img] of order) {
		if (!img || img.w <= 0 || img.h <= 0) continue;
		const need = coverNeed(img, s);
		const enough = floor > 0 ? Math.min(need, floor) : need;
		if (img.w < enough) continue;
		const { url, width } = pickSize(img, kind, need, s);
		return { url, width, need, kind, position: kind === 'poster' ? '50% 20%' : '50% 30%' };
	}
	return null;
}

/**
 * The slides for this screen, in order: every shot sharp enough. When none
 * is (a 5120 px ultrawide), the ones at least ULTRAWIDE_FLOOR wide.
 */
export function playlist(shots: ArtShot[], s: Screen): { shot: ArtShot; pick: ArtPick }[] {
	const strict = shots.flatMap((shot) => {
		const pick = pickArt(shot, s);
		return pick ? [{ shot, pick }] : [];
	});
	if (strict.length) return strict;
	return shots.flatMap((shot) => {
		const pick = pickArt(shot, s, ULTRAWIDE_FLOOR);
		return pick ? [{ shot, pick }] : [];
	});
}

export type Way = 'password' | 'plex' | 'jellyfin' | 'emby' | 'oidc' | 'forward_auth';
export type ProviderButton = { way: Way; label: string };

const LABELS: Record<Way, string> = {
	plex: 'Continue with Plex',
	jellyfin: 'Continue with Jellyfin',
	emby: 'Continue with Emby',
	oidc: 'Continue with single sign-on',
	forward_auth: 'Continue through the trusted proxy',
	password: 'Use a HolyArrs account'
};

/** The buttons the card shows: only the ways the owner turned on, media servers first. */
export function providerButtons(ways: readonly Way[]): ProviderButton[] {
	const order: Way[] = ['plex', 'jellyfin', 'emby', 'oidc', 'forward_auth', 'password'];
	return order.filter((w) => ways.includes(w)).map((way) => ({ way, label: LABELS[way] }));
}

/** One slide, ready to show: backdrop and logo of the same title, both loaded. */
export type ReadySlide = { shot: ArtShot; pick: ArtPick; logo: string | null };

/**
 * Loads the backdrop and the logo of one title together and returns them as
 * one unit, or null when the backdrop fails (the caller moves on). A missing
 * or broken logo gives logo null: the slide shows no logo rather than another
 * title's. Nothing is shown until both loads have settled.
 */
export async function prepareSlide(
	entry: { shot: ArtShot; pick: ArtPick },
	load: (url: string) => Promise<boolean>
): Promise<ReadySlide | null> {
	const logoUrl = typeof entry.shot.logo === 'string' && entry.shot.logo.trim() ? entry.shot.logo : '';
	const [pic, logo] = await Promise.all([load(entry.pick.url), logoUrl ? load(logoUrl) : Promise.resolve(false)]);
	if (!pic) return null;
	return { shot: entry.shot, pick: entry.pick, logo: logo ? logoUrl : null };
}
