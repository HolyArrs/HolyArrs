import { describe, expect, it } from 'vitest';
import { heroFullscreen, heroVideoAllowed, heroVideoStart, pickedSources } from './hero-video.ts';

const base = { setting: true, kid: false, reducedMotion: false };

describe('heroVideoAllowed', () => {
	it('plays on a plain desktop', () => expect(heroVideoAllowed(base)).toBe(true));
	it('never for Kid accounts', () => expect(heroVideoAllowed({ ...base, kid: true, pref: 'always' })).toBe(false));
	it('follows the trailers: reduced motion, data saver and phones stay still unless this device says always', () => {
		expect(heroVideoAllowed({ ...base, reducedMotion: true })).toBe(false);
		expect(heroVideoAllowed({ ...base, saveData: true })).toBe(false);
		expect(heroVideoAllowed({ ...base, phone: true })).toBe(false);
		expect(heroVideoAllowed({ ...base, reducedMotion: true, pref: 'always' })).toBe(true);
	});
	it('stays off with trailers off, never, or a slow connection', () => {
		expect(heroVideoAllowed({ ...base, setting: false })).toBe(false);
		expect(heroVideoAllowed({ ...base, pref: 'never' })).toBe(false);
		expect(heroVideoAllowed({ ...base, effectiveType: '3g' })).toBe(false);
	});
});

describe('heroVideoStart', () => {
	it('keeps the start inside the video', () => {
		expect(heroVideoStart({ start: 63.5, duration: 200 })).toBe(63.5);
		expect(heroVideoStart({ start: 300, duration: 200 })).toBe(0);
		expect(heroVideoStart({ start: -1 })).toBe(0);
	});
});

describe('pickedSources', () => {
	it('plays a picked video through the relay first, the embed last (label videos refuse the embed)', () => {
		expect(pickedSources('agu22bqGHto', { key: 'agu22bqGHto', proxy_url: '/api/v1/trailer/stream/ab/manifest.m3u8' })).toEqual([
			{ kind: 'pass', url: '/api/v1/trailer/stream/ab/manifest.m3u8', key: 'agu22bqGHto' },
			{ kind: 'embed', key: 'agu22bqGHto' }
		]);
	});
	it('falls back to the embed alone without a stream, or with a stream for another video', () => {
		expect(pickedSources('agu22bqGHto', null)).toEqual([{ kind: 'embed', key: 'agu22bqGHto' }]);
		expect(pickedSources('agu22bqGHto', { key: 'WJW-VvmRKsE', proxy_url: '/x.m3u8' })).toEqual([{ kind: 'embed', key: 'agu22bqGHto' }]);
	});
});

describe('heroFullscreen', () => {
	const box = { requestFullscreen: () => Promise.resolve() };
	const video = { webkitEnterFullscreen: () => {} };
	it('uses the browser full screen on the hero where pages may (desktop, Android, iPad)', () => expect(heroFullscreen(true, box, video)).toBe('page'));
	it('uses the video player full screen where pages may not (iPhone Safari)', () => {
		expect(heroFullscreen(undefined, {}, video)).toBe('video');
		expect(heroFullscreen(false, box, video)).toBe('video');
	});
	it('has none without either', () => expect(heroFullscreen(false, {}, {})).toBe(null));
});
