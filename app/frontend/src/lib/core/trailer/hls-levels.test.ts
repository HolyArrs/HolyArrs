import { describe, expect, it } from 'vitest';
import { originalAudio, planLevels, playerTarget } from './hls-player.ts';

describe('relay stream levels', () => {
	it('keeps H.264 only and starts at the tallest up to 720p', () => {
		const levels = [
			{ codec: 'avc1.4D400C', height: 144 },
			{ codec: 'vp09.00.10.08', height: 144 },
			{ codec: 'avc1.4D401F', height: 720 },
			{ codec: 'vp09.00.40.08', height: 1080 },
			{ codec: 'avc1.640028', height: 1080 }
		];
		const p = planLevels(levels);
		expect(p.drop).toEqual([3, 1]);
		// left: 144 avc, 720 avc, 1080 avc -> 720 is index 1
		expect(p.start).toBe(1);
	});
	it('starts at and caps to the tallest H.264 level under the player target', () => {
		const levels = [
			{ codec: 'avc1.4D400C', height: 360 },
			{ codec: 'avc1.4D401F', height: 720 },
			{ codec: 'vp09.00.40.08', height: 1080 },
			{ codec: 'avc1.640028', height: 1080 },
			{ codec: 'avc1.640033', height: 2160 }
		];
		expect(planLevels(levels, 1080)).toEqual({ drop: [2], start: 2 });
		expect(planLevels(levels, 665)).toEqual({ drop: [2], start: 1 });
		expect(planLevels(levels, 900).start).toBe(2);
		// Never above 1080, even when asked.
		expect(planLevels(levels, 4000).start).toBe(2);
		expect(planLevels(levels, 200).start).toBe(0);
		expect(planLevels([{ codec: 'avc1', height: 480 }, { codec: 'avc1', height: 2160 }], 1080).start).toBe(0);
	});
	it('sizes the target to the player, capped at 1080', () => {
		expect(playerTarget(3440, 1440, 'cover', 1)).toBe(1080);
		expect(playerTarget(1536, 600, 'cover', 1)).toBe(864);
		expect(playerTarget(1536, 864, 'contain', 1)).toBe(864);
		expect(playerTarget(640, 300, 'cover', 2)).toBe(720);
	});
	it('leaves a stream without H.264 alone', () => {
		expect(planLevels([{ codec: 'vp09', height: 720 }])).toEqual({ drop: [], start: -1 });
	});
	it('prefers the original audio over auto-dubs', () => {
		const t = [
			{ name: 'Deutsch - dubbed-auto', isDefault: false },
			{ name: 'English original', isDefault: true },
			{ name: 'es-US - dubbed-auto', isDefault: false }
		];
		expect(originalAudio(t)).toBe(1);
		expect(originalAudio([{ name: 'fr - dubbed-auto', isDefault: true }, { name: 'Default', isDefault: false }])).toBe(1);
	});
});

describe('screenTarget', () => {
	it('sizes by the screen, not a small window', async () => {
		const { screenTarget } = await import('./hls-player.ts');
		const g = globalThis as unknown as { screen?: { width: number; height: number }; innerWidth: number; innerHeight: number };
		const was = { screen: g.screen, w: g.innerWidth, h: g.innerHeight };
		Object.defineProperty(globalThis, 'screen', { value: { width: 3440, height: 1440 }, configurable: true });
		g.innerWidth = 1100;
		g.innerHeight = 700;
		expect(screenTarget('contain', 1)).toBe(1080);
		Object.defineProperty(globalThis, 'screen', { value: was.screen, configurable: true });
		g.innerWidth = was.w;
		g.innerHeight = was.h;
	});
});

describe('quick start, then climb', () => {
	it('caps at what is shown', async () => {
		const { capLevel } = await import('./hls-player.ts');
		const hs = [144, 240, 360, 480, 720, 1080];
		expect(hs[capLevel(hs, 700)]).toBe(720);
		expect(hs[capLevel(hs, 1440)]).toBe(1080);
		expect(hs[capLevel(hs, 300)]).toBe(360);
	});
});
