import { describe, expect, it } from 'vitest';
import { clickSources, embedKey, hasSound, onUnmute, playable, sourcesFor, type TrailerSource } from './autoplay.svelte.ts';

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
	it('starts on the relay when the direct stream is picture-only (Chrome), so sound is a plain unmute', () => {
		const out = sourcesFor(s, undefined, false, false);
		expect(out.map((o) => o.kind)).toEqual(['pass', 'direct', 'embed']);
		expect(out[1]).toMatchObject({ url: s.mp4_url, audio: false, pass: s.proxy_url });
		expect(onUnmute(out[0])).toBe('unmute');
	});
	it('switches to pass-through on unmute only for a picture-only stream', () => {
		const direct = sourcesFor(s, undefined, false, false)[1];
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

describe('embedKey', () => {
	const videos = [{ key: 'ownPick0001', type: 'Trailer', official: true, language: 'en' }];
	it('picks from the title videos when the server gave no reason', () => {
		expect(embedKey(undefined, videos, 'en')).toBe('ownPick0001');
	});
	it('plays the key the server names', () => {
		expect(embedKey({ reason: 'bot_check', key: 'serverKey01' }, videos, 'en')).toBe('serverKey01');
		expect(embedKey({ reason: 'age_restricted', key: 'nextKey0001' }, videos, 'en')).toBe('nextKey0001');
	});
	it('leaves the embed out when every trailer will not play here', () => {
		for (const reason of ['age_restricted', 'unavailable', 'sign_in', 'unplayable', 'content_check']) {
			expect(embedKey({ reason, key: '' }, videos, 'en')).toBeUndefined();
		}
		expect(sourcesFor(null, embedKey({ reason: 'age_restricted', key: '' }, videos, 'en'), false, false)).toEqual([]);
	});
	it("keeps its own pick when only YouTube's door failed and no key came", () => {
		expect(embedKey({ reason: 'network', key: '' }, videos, 'en')).toBe('ownPick0001');
	});
	it('puts the reason on the embed when there is no stream, never beside a stream', () => {
		expect(sourcesFor(null, 'nextKey0001', false, false, 'age_restricted')).toStrictEqual([{ kind: 'embed', key: 'nextKey0001', why: 'age_restricted' }]);
		expect(sourcesFor(null, 'nextKey0001', false, false, '')).toStrictEqual([{ kind: 'embed', key: 'nextKey0001' }]);
		const mp4 = { type: 'mp4' as const, url: 'https://rr1.googlevideo.com/videoplayback?itag=18', expires_at: '2099-01-01T00:00:00Z', key: 'abcdefghijk' };
		expect(sourcesFor(mp4, undefined, false, false, 'age_restricted').at(-1)).toStrictEqual({ kind: 'embed', key: 'abcdefghijk' });
	});
});

describe('clickSources', () => {
	const videos = [{ key: 'ownPick0001', type: 'Trailer', official: true, language: 'en' }];
	const mp4: TrailerSource = { kind: 'direct', url: 'https://rr1.googlevideo.com/videoplayback?itag=18', audio: false };
	it('keeps the sources when there are any', () => {
		expect(clickSources([mp4], 'age_restricted', videos, 'en')).toStrictEqual([mp4]);
	});
	it('opens the title trailer with the reason when nothing plays here', () => {
		const s = sourcesFor(null, embedKey({ reason: 'age_restricted', key: '' }, videos, 'en'), false, false, 'age_restricted');
		expect(s).toEqual([]);
		expect(clickSources(s, 'age_restricted', videos, 'en')).toStrictEqual([{ kind: 'embed', key: 'ownPick0001', why: 'age_restricted', told: true }]);
		// YouTube's door failing is not about the video: the player still tries.
		expect(clickSources([], 'bot_check', videos, 'en')).toStrictEqual([{ kind: 'embed', key: 'ownPick0001', why: 'bot_check' }]);
		expect(clickSources([], '', videos, 'en')).toStrictEqual([{ kind: 'embed', key: 'ownPick0001' }]);
	});
	it('stays empty with no trailer at all', () => {
		expect(clickSources([], 'unavailable', [], 'en')).toEqual([]);
		expect(clickSources([], 'unavailable', undefined, 'en')).toEqual([]);
	});
});
