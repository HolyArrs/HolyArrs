import { afterEach, describe, expect, it, vi } from 'vitest';
import FakeHls from './hls-fake.ts';

vi.mock('../paths.ts', () => ({ to: () => new URL('./hls-fake.ts', import.meta.url).href }));

const { attachHLS } = await import('./hls-player.ts');

vi.stubGlobal('document', { addEventListener() {}, removeEventListener() {}, fullscreenElement: null });
vi.stubGlobal('window', { addEventListener() {}, removeEventListener() {} });

afterEach(() => (FakeHls.made.length = 0));

const video = (h: number) => ({ clientWidth: (h * 16) / 9, clientHeight: h, dataset: {}, dispatchEvent() {} }) as unknown as HTMLVideoElement;

describe('attachHLS quality', () => {
	it('starts at the level the player shows and leaves quality switching on', async () => {
		const stop = await attachHLS(video(700), '/api/v1/trailer/stream/x/manifest.m3u8', false);
		const hls = FakeHls.made[0];
		hls.emit(FakeHls.Events.MANIFEST_PARSED);
		expect(hls.levels[hls.startLevel].height).toBe(720);
		expect(hls.autoLevelCapping).toBe(hls.startLevel);
		expect(hls.autoLevelEnabled).toBe(true);
		stop();
	});
});
