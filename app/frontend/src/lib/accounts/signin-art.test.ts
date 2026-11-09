import { coverNeed, pickArt, playlist, prepareSlide, providerTiles, sized, type ArtShot } from './signin-art.ts';
import { describe, expect, it, vi } from 'vitest';

const B = 'https://image.tmdb.org/t/p/original/';
const shot = (bw: number, pw?: number): ArtShot => ({
	backdrop: { url: B + 'b.jpg', w: bw, h: Math.round((bw * 9) / 16) },
	poster: pw ? { url: B + 'p.jpg', w: pw, h: pw * 1.5 } : undefined,
	logo: 'https://image.tmdb.org/t/p/w500/l.png'
});

describe('coverNeed', () => {
	it('is the viewport width times DPR for a wide screen', () => {
		expect(coverNeed({ w: 3840, h: 2160 }, { vw: 1920, vh: 1080, dpr: 1 })).toBe(1920);
		expect(coverNeed({ w: 3840, h: 2160 }, { vw: 3440, vh: 1440, dpr: 1 })).toBe(3440);
	});
	it('follows the height on a tall screen', () => {
		// 390x844 at DPR 3: a 16:9 backdrop must be 844*16/9*3 wide.
		expect(coverNeed({ w: 3840, h: 2160 }, { vw: 390, vh: 844, dpr: 3 })).toBe(4502);
	});
});

describe('sized', () => {
	it('swaps the TMDB size', () => {
		expect(sized(B + 'x.jpg', 1280)).toBe('https://image.tmdb.org/t/p/w1280/x.jpg');
		expect(sized('https://image.tmdb.org/t/p/w780/x.jpg', 'original')).toBe(B + 'x.jpg');
	});
});

describe('pickArt', () => {
	it('1920x1080 at DPR 1 takes the original, with w1280 first', () => {
		const p = pickArt(shot(3840), { vw: 1920, vh: 1080, dpr: 1 })!;
		expect(p.url).toBe(B + 'b.jpg');
		expect(p.width).toBe(3840);
		expect(p.low).toBe('https://image.tmdb.org/t/p/w1280/b.jpg');
	});
	it('1440x900 at DPR 2 takes the original', () => {
		expect(pickArt(shot(3840), { vw: 1440, vh: 900, dpr: 2 })!.url).toBe(B + 'b.jpg');
	});
	it('390 px wide at DPR 3 (1170 px) takes w1280', () => {
		const p = pickArt(shot(3840), { vw: 390, vh: 200, dpr: 3 })!;
		expect(p.url).toContain('/w1280/');
		expect(p.low).toBeUndefined();
	});
	it('the size covers the screen at the end of the slow zoom', () => {
		// 1200 * 1.06 = 1272: w1280 still covers it; 1220 * 1.06 does not.
		expect(pickArt(shot(3840), { vw: 1200, vh: 600, dpr: 1 })!.width).toBe(1280);
		expect(pickArt(shot(3840), { vw: 1220, vh: 600, dpr: 1 })!.width).toBe(3840);
	});
	it('a small DPR 1 window takes a TMDB size that still covers it', () => {
		const p = pickArt(shot(3840), { vw: 1100, vh: 600, dpr: 1 })!;
		expect(p.url).toContain('/w1280/');
		expect(p.width).toBeGreaterThanOrEqual(p.need);
	});
	it('3440x1440 ultrawide skips a 1920 backdrop and takes a 3840 one', () => {
		const s = { vw: 3440, vh: 1440, dpr: 1 };
		expect(pickArt(shot(1920), s)).toBeNull();
		const p = pickArt(shot(3840), s)!;
		expect(p.url).toBe(B + 'b.jpg');
	});
	it('5120x1440 takes a 5120 backdrop, and only falls back to 3840 when none is sharp enough', () => {
		const s = { vw: 5120, vh: 1440, dpr: 1 };
		expect(pickArt(shot(3840), s)).toBeNull();
		expect(pickArt(shot(5120), s)!.width).toBe(5120);
		const list = playlist([shot(1920), shot(3840)], s);
		expect(list).toHaveLength(1);
		expect(list[0].shot.backdrop.w).toBe(3840);
		expect(playlist([shot(3840), shot(5120)], s).map((x) => x.shot.backdrop.w)).toEqual([5120]);
	});
	it('iPhone 390x844 at DPR 3 uses the poster', () => {
		const p = pickArt(shot(3840, 2000), { vw: 390, vh: 844, dpr: 3 })!;
		expect(p.kind).toBe('poster');
		expect(p.url).toBe('https://image.tmdb.org/t/p/original/p.jpg');
	});
	it('iPhone without a sharp poster skips the title instead of stretching', () => {
		expect(pickArt(shot(3840, 780), { vw: 390, vh: 844, dpr: 3 })).toBeNull();
		expect(pickArt(shot(3840), { vw: 390, vh: 844, dpr: 3 })).toBeNull();
	});
});

describe('prepareSlide: backdrop and logo switch as one unit', () => {
	const totoro: ArtShot = { backdrop: { url: 'https://image.tmdb.org/t/p/original/totoro.jpg', w: 3840, h: 2160 }, logo: 'https://image.tmdb.org/t/p/original/totoro-logo.png' };
	const entry = { shot: totoro, pick: pickArt(totoro, { vw: 1920, vh: 1080, dpr: 1 })! };

	it('preloads both the picture and the logo before resolving', async () => {
		const loaded: string[] = [];
		let release!: () => void;
		const gate = new Promise<void>((r) => (release = r));
		const load = async (url: string) => {
			loaded.push(url);
			if (url.includes('logo')) await gate;
			return true;
		};
		let done = false;
		const p = prepareSlide(entry, load).then((s) => ((done = true), s));
		await Promise.resolve();
		expect(loaded).toEqual([entry.pick.url, totoro.logo]);
		await new Promise((r) => setTimeout(r, 0));
		expect(done).toBe(false); // Still waiting on the logo.
		release();
		const s = await p;
		expect(s?.pick.url).toBe(entry.pick.url);
		expect(s?.logo).toBe(totoro.logo);
		expect(s?.shot).toBe(totoro);
	});

	it('shows no logo when the title has none, never another title\'s', async () => {
		const bare = { ...entry, shot: { ...totoro, logo: '' } };
		const load = vi.fn(async () => true);
		const s = await prepareSlide(bare, load);
		expect(s?.logo).toBeNull();
		expect(load).toHaveBeenCalledTimes(1);
	});

	it('drops a broken logo but keeps the picture', async () => {
		const s = await prepareSlide(entry, async (u) => !u.includes('logo'));
		expect(s?.logo).toBeNull();
		expect(s?.pick.url).toBe(entry.pick.url);
	});

	it('skips the slide when the picture fails', async () => {
		expect(await prepareSlide(entry, async (u) => u.includes('logo'))).toBeNull();
	});
});

describe('providerTiles', () => {
	it('shows only the enabled outside ways, in order, with short names', () => {
		const t = providerTiles(['password', 'oidc', 'plex', 'emby']);
		expect(t.map((x) => x.way)).toEqual(['plex', 'emby', 'oidc']);
		expect(t.map((x) => x.label)).toEqual(['Plex', 'Emby', 'Single sign-on']);
	});
	it('a single tile says Continue with', () => {
		expect(providerTiles(['jellyfin', 'password'])).toEqual([{ way: 'jellyfin', name: 'Jellyfin', label: 'Continue with Jellyfin' }]);
	});
	it('password only gives no tiles', () => {
		expect(providerTiles(['password'])).toEqual([]);
	});
});

import { easeOut, waveLevel, WAVE_DONE_MS, WAVE_JITTER_MS, WAVE_MS } from './signin-art.ts';
describe('radial wave', () => {
	it('starts as the first-frame mosaic everywhere', () => {
		for (const f of [0, 0.5, 1]) expect(waveLevel(0, f, -WAVE_JITTER_MS)).toBe(0);
	});
	it('sharpens near the panel first, then rings outward', () => {
		const mid = WAVE_MS * 0.3;
		expect(waveLevel(mid, 0)).toBeGreaterThan(waveLevel(mid, 0.6));
		expect(waveLevel(mid, 0.6)).toBeGreaterThanOrEqual(waveLevel(mid, 1));
	});
	it('only ever moves towards sharp, one size at a time', () => {
		for (const f of [0, 0.3, 0.7, 1]) {
			let last = 0;
			for (let t = 0; t <= WAVE_DONE_MS; t += 4) {
				const l = waveLevel(t, f);
				expect(l - last).toBeGreaterThanOrEqual(0);
				expect(l - last).toBeLessThanOrEqual(1);
				last = l;
			}
		}
	});
	it('is sharp everywhere once done, jitter included', () => {
		for (const f of [0, 0.5, 1]) for (const j of [-WAVE_JITTER_MS, 0, WAVE_JITTER_MS]) expect(waveLevel(WAVE_DONE_MS, f, j)).toBe(4);
	});
	it('eases out', () => {
		expect(easeOut(0.5)).toBeGreaterThan(0.8);
		expect(easeOut(2)).toBe(1);
	});
});

import { nextOrigin, swapEase, interacting, SWAP_ORIGINS, IDLE_MS } from './signin-art.ts';
describe('swap helpers', () => {
	it('never picks the same origin twice in a row', () => {
		let last = -1;
		for (let i = 0; i < 500; i++) {
			const o = nextOrigin(last);
			expect(o).not.toBe(last);
			expect(o).toBeGreaterThanOrEqual(0);
			expect(o).toBeLessThan(SWAP_ORIGINS.length);
			last = o;
		}
		expect(nextOrigin(3, () => 0.9999)).not.toBe(3);
	});
	it('eases fast then settles', () => {
		expect(swapEase(0)).toBe(0);
		expect(swapEase(1)).toBe(1);
		expect(swapEase(0.25)).toBeGreaterThan(0.5);
	});
	it('pauses while typing and for 5 s after', () => {
		expect(interacting(10000, 9000, false, true)).toBe(true);
		expect(interacting(10000 + IDLE_MS, 10000, false, true)).toBe(false);
		expect(interacting(99999, 0, true, true)).toBe(true);
		expect(interacting(99999, 0, true, false)).toBe(false);
	});
});

import { alreadyLinked, inviteTiles } from './signin-art.ts';
describe('inviteTiles', () => {
	it('offers only Plex, Jellyfin and Emby on an invite', () => {
		expect(inviteTiles(['password', 'oidc', 'emby', 'plex', 'forward_auth']).map((t) => t.way)).toEqual(['plex', 'emby']);
		expect(inviteTiles(['password', 'oidc'])).toEqual([]);
		expect(inviteTiles(['jellyfin'])[0].label).toBe('Continue with Jellyfin');
	});
	it('knows the already-linked answer', () => {
		expect(alreadyLinked({ status: 409, body: { code: 'linked' } })).toBe(true);
		expect(alreadyLinked({ status: 409, body: { error: 'that name is taken' } })).toBe(false);
		expect(alreadyLinked(new Error('x'))).toBe(false);
		expect(alreadyLinked(null)).toBe(false);
	});
});
