import { describe, expect, it } from 'vitest';
import { playable } from './autoplay.svelte.ts';

describe('playable', () => {
	const hls = { type: 'hls' as const, url: 'https://manifest.googlevideo.com/api/manifest/hls_variant/x', mp4_url: 'https://rr1.googlevideo.com/videoplayback?itag=18' };
	it('plays the manifest only with native HLS', () => {
		expect(playable(hls, true)).toBe(hls.url);
		expect(playable(hls, false)).toBe(hls.mp4_url);
	});
	it('falls back to nothing without an MP4 beside the manifest', () => {
		expect(playable({ type: 'hls', url: hls.url }, false)).toBeNull();
	});
	it('plays an MP4 answer anywhere', () => {
		expect(playable({ type: 'mp4', url: hls.mp4_url }, true)).toBe(hls.mp4_url);
		expect(playable({ url: hls.mp4_url }, false)).toBe(hls.mp4_url);
	});
});
