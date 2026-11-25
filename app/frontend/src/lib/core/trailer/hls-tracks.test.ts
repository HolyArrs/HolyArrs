import { afterEach, describe, expect, it, vi } from 'vitest';
import FakeHls from './hls-fake.ts';

vi.mock('../paths.ts', () => ({ to: () => new URL('./hls-fake.ts', import.meta.url).href }));

const { attachHLS } = await import('./hls-player.ts');

vi.stubGlobal('document', { addEventListener() {}, removeEventListener() {}, fullscreenElement: null });
vi.stubGlobal('window', { addEventListener() {}, removeEventListener() {} });

afterEach(() => (FakeHls.made.length = 0));

const video = { clientWidth: 1280, clientHeight: 720, dataset: {}, dispatchEvent() {} } as unknown as HTMLVideoElement;

// every <track> hls.js makes has src "data:,WEBVTT", which media-src blocks.
describe('attachHLS text tracks', () => {
	it('asks hls.js for no <track> elements', async () => {
		const stop = await attachHLS(video, '/api/v1/trailer/stream/x/manifest.m3u8', false);
		expect(FakeHls.made[0].config).toMatchObject({
			renderTextTracksNatively: false,
			enableID3MetadataCues: false,
			enableEmsgMetadataCues: false,
			enableDateRangeMetadataCues: false
		});
		stop();
	});
});
