import { describe, expect, it, vi } from 'vitest';
import { claimPlayer, dwell, embedUrl, heroPictures, mayAutoplay, pickTrailer, placeCard, playingWord, type Video } from './trailer.ts';

const vs: Video[] = [
	{ key: 'teaserEN01', type: 'Teaser', official: true, language: 'en' },
	{ key: 'fanTrailer1', type: 'Trailer', official: false, language: 'en' },
	{ key: 'officialEN1', type: 'Trailer', official: true, language: 'en' },
	{ key: 'officialDE1', type: 'Trailer', official: true, language: 'de' }
];

describe('pickTrailer', () => {
	it('prefers an official trailer in the person language, then English', () => {
		expect(pickTrailer(vs, 'en')?.key).toBe('officialEN1');
		expect(pickTrailer(vs, 'de-DE')?.key).toBe('officialDE1');
		expect(pickTrailer(vs, 'fr')?.key).toBe('officialEN1');
	});
	it('falls back to a teaser, never a clip or odd key', () => {
		expect(pickTrailer(vs.slice(0, 1))?.key).toBe('teaserEN01');
		expect(pickTrailer([{ key: 'clip123456', type: 'Clip' }, { key: 'bad key!', type: 'Trailer' }])).toBeNull();
		expect(pickTrailer(undefined)).toBeNull();
	});
});

describe('mayAutoplay', () => {
	const base = { setting: true, kid: false, reducedMotion: false };
	it('plays only when every condition allows', () => {
		expect(mayAutoplay(base)).toBe(true);
		expect(mayAutoplay({ ...base, effectiveType: '4g' })).toBe(true);
		expect(mayAutoplay({ ...base, setting: false })).toBe(false);
		expect(mayAutoplay({ ...base, kid: true })).toBe(false);
		expect(mayAutoplay({ ...base, reducedMotion: true })).toBe(false);
		expect(mayAutoplay({ ...base, saveData: true })).toBe(false);
		// Phones never autoplay: the Trailer button opens the theater on a tap.
		expect(mayAutoplay({ ...base, phone: true })).toBe(false);
		expect(mayAutoplay({ ...base, phone: false })).toBe(true);
		for (const t of ['slow-2g', '2g', '3g']) expect(mayAutoplay({ ...base, effectiveType: t })).toBe(false);
	});
	it('never plays in the minimal look, even when the person picked always', () => {
		expect(mayAutoplay({ ...base, minimal: true })).toBe(false);
		expect(mayAutoplay({ ...base, minimal: true, pref: 'always' })).toBe(false);
		expect(mayAutoplay({ ...base, minimal: false, pref: 'always' })).toBe(true);
	});
});

describe('dwell', () => {
	it('fires after the delay unless cancelled, and restarts', () => {
		vi.useFakeTimers();
		const fire = vi.fn();
		const d = dwell(800, fire);
		d.start();
		vi.advanceTimersByTime(799);
		d.start();
		vi.advanceTimersByTime(799);
		expect(fire).not.toHaveBeenCalled();
		vi.advanceTimersByTime(1);
		expect(fire).toHaveBeenCalledOnce();
		d.start();
		d.cancel();
		vi.advanceTimersByTime(2000);
		expect(fire).toHaveBeenCalledOnce();
		expect(d.pending).toBe(false);
		vi.useRealTimers();
	});
});

describe('claimPlayer', () => {
	it('keeps one video playing', () => {
		const a = vi.fn();
		const releaseA = claimPlayer(a);
		const releaseB = claimPlayer(() => {});
		expect(a).toHaveBeenCalledOnce();
		releaseA();
		releaseB();
	});
});

describe('helpers', () => {
	it('builds the privacy-enhanced embed', () => {
		const u = new URL(embedUrl('abcDEF123', 'http://10.0.0.1'));
		expect(u.host).toBe('www.youtube-nocookie.com');
		expect(u.pathname).toBe('/embed/abcDEF123');
		for (const [k, v] of Object.entries({ autoplay: '1', mute: '1', controls: '0', playsinline: '1', loop: '1', modestbranding: '1', rel: '0' })) expect(u.searchParams.get(k)).toBe(v);
	});
	it('lists hero pictures without repeats, at most 4', () => {
		expect(heroPictures('a', ['a', 'b', 'c', 'd', 'e'])).toEqual(['a', 'b', 'c', 'd']);
		expect(heroPictures(null, [])).toEqual([]);
	});
	it('keeps the hover card inside the viewport', () => {
		const view = { width: 1000, height: 800 };
		expect(placeCard({ left: 0, top: 10, width: 100, height: 150 }, 300, 300, view)).toEqual({ left: 8, top: 8 });
		// At a row's left edge it lines up with the poster, not the window edge.
		expect(placeCard({ left: 165, top: 300, width: 170, height: 255 }, 300, 300, view, 8, { left: 160, right: 1000 }).left).toBe(165);
		expect(placeCard({ left: 950, top: 700, width: 50, height: 100 }, 300, 300, view)).toEqual({ left: 692, top: 492 });
		expect(placeCard({ left: 820, top: 300, width: 170, height: 255 }, 300, 300, view, 8, { left: 160, right: 1000 }).left).toBe(690);
		expect(placeCard({ left: 400, top: 300, width: 200, height: 200 }, 300, 300, view)).toEqual({ left: 350, top: 250 });
	});
});

describe('playingWord', () => {
	it('names what the big player plays', () => {
		expect(playingWord('artist')).toBe('music video');
		expect(playingWord('movie')).toBe('trailer');
		expect(playingWord('show', 'teaser')).toBe('teaser');
	});
});
