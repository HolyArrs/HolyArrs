import { describe, expect, it } from 'vitest';
import { originalAudio, planLevels } from './hls-player.ts';

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
