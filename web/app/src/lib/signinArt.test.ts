import { describe, expect, it, vi } from 'vitest';
import { coverNeed, pickArt, playlist, prepareSlide, providerButtons, providerTiles, sized, type ArtShot } from './signinArt.ts';

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
	it('1920x1080 at DPR 1 takes the original (over 1280 needed)', () => {
		const p = pickArt(shot(3840), { vw: 1920, vh: 1080, dpr: 1 })!;
		expect(p.url).toBe(B + 'b.jpg');
		expect(p.width).toBeGreaterThanOrEqual(p.need);
	});
	it('a small DPR 1 window takes a TMDB size that still covers it', () => {
		const p = pickArt(shot(3840), { vw: 1200, vh: 700, dpr: 1 })!;
		expect(p.url).toContain('/w1280/');
		expect(p.width).toBeGreaterThanOrEqual(p.need);
	});
	it('3440x1440 ultrawide skips a 1920 backdrop and takes a 3840 one', () => {
		const s = { vw: 3440, vh: 1440, dpr: 1 };
		expect(pickArt(shot(1920), s)).toBeNull();
		const p = pickArt(shot(3840), s)!;
		expect(p.url).toBe(B + 'b.jpg');
		expect(p.width).toBeGreaterThanOrEqual(3440);
	});
	it('5120x1440 takes a 5120 backdrop, and only falls back to 3840 when none is sharp enough', () => {
		const s = { vw: 5120, vh: 1440, dpr: 1 };
		expect(pickArt(shot(3840), s)).toBeNull();
		expect(pickArt(shot(5120), s)!.width).toBe(5120);
		const list = playlist([shot(1920), shot(3840)], s);
		expect(list).toHaveLength(1);
		expect(list[0].pick.width).toBe(3840);
		expect(playlist([shot(3840), shot(5120)], s).map((x) => x.pick.width)).toEqual([5120]);
	});
	it('iPhone 390x844 at DPR 3 uses the poster', () => {
		const p = pickArt(shot(3840, 2000), { vw: 390, vh: 844, dpr: 3 })!;
		expect(p.kind).toBe('poster');
		expect(p.url).toBe(B + 'p.jpg');
		expect(p.width).toBeGreaterThanOrEqual(p.need);
	});
	it('iPhone without a sharp poster skips the title instead of stretching', () => {
		expect(pickArt(shot(3840, 780), { vw: 390, vh: 844, dpr: 3 })).toBeNull();
		expect(pickArt(shot(3840), { vw: 390, vh: 844, dpr: 3 })).toBeNull();
	});
});

describe('providerButtons', () => {
	it('shows only the ways that are on, media servers first, HolyArrs account last', () => {
		expect(providerButtons(['password']).map((b) => b.label)).toEqual(['Use a HolyArrs account']);
		expect(providerButtons(['password', 'oidc', 'emby', 'plex', 'jellyfin']).map((b) => b.label)).toEqual([
			'Continue with Plex',
			'Continue with Jellyfin',
			'Continue with Emby',
			'Continue with single sign-on',
			'Use a HolyArrs account'
		]);
		expect(providerButtons(['plex']).map((b) => b.way)).toEqual(['plex']);
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
		expect(done).toBe(false); // still waiting on the logo
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
