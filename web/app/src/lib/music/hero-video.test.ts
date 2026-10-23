import { describe, expect, it } from 'vitest';
import { heroVideoAllowed, heroVideoStart } from './hero-video.ts';

const base = { setting: true, kid: false, reducedMotion: false };

describe('heroVideoAllowed', () => {
	it('plays on a plain desktop', () => expect(heroVideoAllowed(base)).toBe(true));
	it('never for Kid accounts', () => expect(heroVideoAllowed({ ...base, kid: true, pref: 'always' })).toBe(false));
	it('respects reduced motion and data saver, even with always', () => {
		expect(heroVideoAllowed({ ...base, reducedMotion: true, pref: 'always' })).toBe(false);
		expect(heroVideoAllowed({ ...base, saveData: true, pref: 'always' })).toBe(false);
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
