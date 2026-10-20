import { describe, expect, it } from 'vitest';
import { hasSound, onUnmute, playable, sourcesFor, type TrailerSource } from './autoplay.svelte.ts';

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

describe('pass-through', () => {
	const s = {
		type: 'hls' as const,
		url: 'https://manifest.googlevideo.com/api/manifest/hls_variant/x',
		mp4_url: 'https://rr1.googlevideo.com/videoplayback?itag=136',
		mp4_audio: false,
		audio: true,
		expires_at: '2099-01-01T00:00:00Z',
		key: 'abcdefghijk',
		proxy_url: '/api/v1/trailer/stream/0a/manifest.m3u8'
	};
	it('starts direct and muted, with the relay as the fallback (Chrome)', () => {
		const out = sourcesFor(s, undefined, false, false);
		expect(out.map((o) => o.kind)).toEqual(['direct', 'pass', 'embed']);
		expect(out[0]).toMatchObject({ url: s.mp4_url, audio: false, pass: s.proxy_url });
	});
	it('switches to pass-through on unmute only for a picture-only stream', () => {
		const [direct] = sourcesFor(s, undefined, false, false);
		expect(onUnmute(direct)).toBe('switch');
		expect(hasSound(direct)).toBe(true);
		expect(onUnmute({ ...direct, pass: undefined } as TrailerSource)).toBe('none');
		expect(onUnmute({ kind: 'direct', url: 'x', audio: true })).toBe('unmute');
		expect(onUnmute({ kind: 'embed', key: 'k' })).toBe('unmute');
	});
	it('starts on pass-through once sound was turned on this session', () => {
		expect(sourcesFor(s, undefined, false, true).map((o) => o.kind)).toEqual(['pass', 'direct', 'embed']);
	});
	it('keeps native HLS direct in Safari, sound or not', () => {
		for (const sound of [false, true]) {
			const out = sourcesFor(s, undefined, true, sound);
			expect(out.map((o) => o.kind)).toEqual(['direct', 'direct', 'pass', 'embed']);
			expect(out[0]).toMatchObject({ url: s.url, audio: true });
			expect(onUnmute(out[0])).toBe('unmute');
		}
	});
	it('has no relay for an MP4 answer', () => {
		expect(sourcesFor({ ...s, type: 'mp4', url: s.mp4_url, proxy_url: undefined }, 'vk', false, true).map((o) => o.kind)).toEqual(['direct', 'embed']);
		expect(sourcesFor(null, 'vk', false, true)).toEqual([{ kind: 'embed', key: 'vk' }]);
	});
});
