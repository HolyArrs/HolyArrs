// Sign-in art: which picture, at which size, for this screen. The server answers same-origin picture addresses (its artwork cache, since TMDB sends no CORS header and the reveal draws on a canvas) with each picture's native size; this file picks the served size for the screen it is shown on.

import { t } from '#lib/core/i18n/i18n.svelte.ts';

export type ArtImage = { url: string; w: number; h: number };
export type ArtShot = { backdrop: ArtImage; poster?: ArtImage; logo: string };
export type Screen = { vw: number; vh: number; dpr: number };

export type ArtPick = {
	/** The address to load (one of the served sizes). */
	url: string;
	/** A lighter size shown first while url loads (the first slide), then crossfaded away. */
	low?: string;
	/** Native width of the chosen size, in pixels. */
	width: number;
	/** Pixels the picture covers on this screen (width), at this DPR. */
	need: number;
	kind: 'backdrop' | 'poster';
	/** object-position: keep faces and the top of the frame. */
	position: string;
};

export type Way = 'password' | 'plex' | 'jellyfin' | 'emby' | 'oidc' | 'forward_auth';

/** One slide, ready to show: backdrop and logo of the same title, both loaded. */
export type ReadySlide = { shot: ArtShot; pick: ArtPick; logo: string | null };

/** A provider tile on the card: the short name when several share the row, the full "Continue with" when one stands alone. */
export type ProviderTile = { way: Exclude<Way, 'password'>; name: string; label: string };

/** Screens narrower than this (width / height) are tall: phones in portrait. */
export const TALL = 0.8;

/** On a wide screen, a picture at least this wide counts as sharp enough when nothing reaches the full need (ultrawide 5120 px screens). */
export const ULTRAWIDE_FLOOR = 3840;

/** The slow zoom's end scale: the picture must stay sharp at it. */
export const ZOOM = 1.06;

/** Served sizes per kind; 0 is the original (backdrops up to 3840 px, posters for dense phones). */
const SIZES: Record<ArtPick['kind'], number[]> = { backdrop: [780, 1280, 0], poster: [500, 780, 0] };

/** Device pixels a cover-fit picture of w x h spans across, on the screen. */
export function coverNeed(img: { w: number; h: number }, s: Screen): number {
	const cssWidth = Math.max(s.vw, (s.vh * img.w) / img.h);
	return Math.ceil(cssWidth * Math.max(1, s.dpr));
}

/** A picture address at another size: /t/p/w780/x.jpg to /t/p/w1280/x.jpg. */
export function sized(url: string, size: number | 'original'): string {
	return url.replace(/\/t\/p\/[^/]+\//, `/t/p/${size === 'original' ? 'original' : 'w' + size}/`);
}

/** The picture for this screen, or null when this title has none sharp enough (the caller moves on to the next title rather than stretching). `floor` relaxes the need for very wide screens: the native width must reach min(need, floor). Pass 0 for the strict gate. */
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
		const { url, width } = pickSize(img, kind, need);
		const low = width > 1280 && kind === 'backdrop' ? sized(img.url, 1280) : undefined;
		return { url, low, width, need, kind, position: kind === 'poster' ? '50% 20%' : '50% 30%' };
	}
	return null;
}

/** The slides for this screen, in order: every shot sharp enough. When none is (a 5120 px ultrawide), the ones at least ULTRAWIDE_FLOOR wide. */
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

const LABELS: Record<Way, string> = {
	plex: 'Continue with Plex',
	jellyfin: 'Continue with Jellyfin',
	emby: 'Continue with Emby',
	oidc: 'Continue with single sign-on',
	forward_auth: 'Continue through the trusted proxy',
	password: 'Use a HolyArrs account'
};

/** Loads the backdrop and the logo of one title together and returns them as one unit, or null when the backdrop fails (the caller moves on). A missing or broken logo gives logo null: the slide shows no logo rather than another title's. */
export async function prepareSlide(
	entry: { shot: ArtShot; pick: ArtPick },
	load: (url: string) => Promise<boolean>
): Promise<ReadySlide | null> {
	const logoUrl = typeof entry.shot.logo === 'string' && entry.shot.logo.trim() ? entry.shot.logo : '';
	const [pic, logo] = await Promise.all([load(entry.pick.url), logoUrl ? load(logoUrl) : Promise.resolve(false)]);
	if (!pic) return null;
	return { shot: entry.shot, pick: entry.pick, logo: logo ? logoUrl : null };
}

const nameOf = (way: Exclude<Way, 'password'>): string => (way === 'oidc' ? t('accounts.tiles.sso') : way === 'forward_auth' ? t('accounts.tiles.proxy') : way === 'plex' ? 'Plex' : way === 'jellyfin' ? 'Jellyfin' : 'Emby');

/** The tile row: only the outside ways the owner turned on, in a fixed order. */
export function providerTiles(ways: readonly Way[]): ProviderTile[] {
	const order = ['plex', 'jellyfin', 'emby', 'oidc', 'forward_auth'] as const;
	const on = order.filter((w) => ways.includes(w));
	return on.map((way) => ({ way, name: nameOf(way), label: on.length > 1 ? nameOf(way) : t('accounts.tiles.continueWith', { name: nameOf(way) }) }));
}

/** The invite page's tiles: the media-server ways only (single sign-on and the proxy make no new people from an invite). */
export function inviteTiles(ways: readonly Way[]): ProviderTile[] {
	return providerTiles(ways.filter((w) => w === 'plex' || w === 'jellyfin' || w === 'emby'));
}

/** True when an invite join failed because that outside account already has a user. */
export function alreadyLinked(err: unknown): boolean {
	const e = err as { status?: number; body?: Record<string, unknown> } | null;
	return e?.status === 409 && e.body?.code === 'linked';
}

/** Width of the kept thumbnail (the reveal's detail source until the full picture is in). */
export const THUMB_W = 64;

/** Blocks across the first-frame mosaic (the 24 px thumbnail app.html paints). */
export const SEED_COLS = 24;

/** Square block sizes (CSS px) the wave steps through after the first-frame mosaic, then sharp. 36 is the Search sweep's cell. */
export const WAVE_BLOCKS = [36, 12, 6] as const;

/** A short still hold on the first frame (the app boots under it). */
export const REVEAL_HOLD_MS = 320;

/** The ring's run from the panel centre to the farthest screen corner. */
export const WAVE_MS = 1800;

/** How wide the ring's sharpening edge is, as a share of that run. */
export const WAVE_BAND = 0.3;

/** Each 36 px cell starts up to this much early or late, so the ring's edge is ragged. */
export const WAVE_JITTER_MS = 75;

/** The wave is sharp everywhere from here on. */
export const WAVE_DONE_MS = WAVE_MS + WAVE_JITTER_MS;

/** Cubic ease-out: a fast start, a long settle. */
export function easeOut(p: number): number {
	const c = Math.min(1, Math.max(0, p));
	return 1 - Math.pow(1 - c, 3);
}

/** The level of one block at `ms` into the wave: 0 the first-frame mosaic, 1 to 3 the WAVE_BLOCKS sizes, 4 sharp. `f` is the block's distance from the panel centre (0) to the farthest corner (1); `jitter` is its offset in ms. The shader in SigninBackdrop.svelte computes the same. */
export function waveLevel(ms: number, f: number, jitter = 0): number {
	const p = easeOut((ms + jitter) / WAVE_MS) * (1 + WAVE_BAND);
	const s = Math.min(1, Math.max(0, (p - f) / WAVE_BAND));
	return s >= 1 ? 4 : Math.floor(s * 4);
}

/** Fired on window (and html[data-signin-gl] set) once the wave shader is compiled or not needed; the panel stagger waits for it. */
export const SIGNIN_GL_EVENT = 'holyarrs:signin-gl';

// Later picture changes: a softer, quicker mosaic dissolve from an edge or corner.
export const CYCLE_MS = 20000;

export const SWAP_MS = 1200;
export const SWAP_BLOCK = 56; // CSS px: larger than the first reveal's 36 px blocks.
export const SWAP_BAND = 0.35;
export const SWAP_JITTER_MS = 90;
export const SWAP_DONE_MS = SWAP_MS + SWAP_JITTER_MS;
export const IDLE_MS = 5000; // The cycle resumes this long after the last interaction.

/** Where a swap's wave starts, as a fraction of the screen (edges and corners). */
export const SWAP_ORIGINS: readonly (readonly [number, number])[] = [
	[0, 0.5], [1, 0.5], [0.5, 0], [0.5, 1], [0, 0], [1, 0], [0, 1], [1, 1]
];

/** A random origin index, never the same as the last one. */
export function nextOrigin(last: number, rnd: () => number = Math.random): number {
	const n = SWAP_ORIGINS.length;
	if (last < 0 || last >= n) return Math.floor(rnd() * n) % n;
	return (last + 1 + (Math.floor(rnd() * (n - 1)) % (n - 1))) % n;
}

/** Swap easing: fast start, slow settle (cubic ease-out). */
export function swapEase(p: number): number {
	const c = Math.min(1, Math.max(0, p));
	return 1 - Math.pow(1 - c, 3);
}

/** True when the user is mid-interaction, so the picture must not change. */
export function interacting(now: number, lastInput: number, fieldFocused: boolean, engaged: boolean): boolean {
	return (engaged && fieldFocused) || now - lastInput < IDLE_MS;
}

function pickSize(img: ArtImage, kind: ArtPick['kind'], need: number): { url: string; width: number } {
	// The smallest served size that covers the screen at the zoom's end, else the largest (the original).
	const fits = SIZES[kind].map((w) => w || img.w).filter((w) => w <= img.w);
	const w = fits.find((x) => x >= Math.ceil(need * ZOOM)) ?? fits.at(-1) ?? SIZES[kind][0];
	const original = w === img.w && !SIZES[kind].includes(w);
	return { url: sized(img.url, original ? 'original' : w), width: w };
}
