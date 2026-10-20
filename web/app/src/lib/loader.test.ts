import { describe, expect, it } from 'vitest';
import { HOLD_MS, NAV_DELAY_MS, backoff, bootHold, isUserSwitch, remaining } from './loader.ts';

describe('loader timing', () => {
	it('holds the screen for the rest of 2.2 s', () => {
		expect(HOLD_MS).toBe(2200);
		expect(remaining(0, 500)).toBe(1700);
	});
	it('leaves at once when the app took longer than the hold', () => {
		expect(remaining(0, 4000)).toBe(0);
	});
	it('counts from a replay, not the page load', () => {
		expect(remaining(10000, 10200)).toBe(2000);
	});
	it('skips fast page changes', () => {
		expect(NAV_DELAY_MS).toBe(300);
	});
});

describe('loader moments', () => {
	it('backs off 1 s, 2 s, 4 s… up to 15 s', () => {
		expect([0, 1, 2, 3, 4, 9].map(backoff)).toEqual([1000, 2000, 4000, 8000, 15000, 15000]);
	});
	it('says welcome only when a different person signs in', () => {
		expect(isUserSwitch(null, 'home')).toBe(false);
		expect(isUserSwitch('home', 'home')).toBe(false);
		expect(isUserSwitch('home', 'kid')).toBe(true);
	});
});

describe('boot hold', () => {
	it('holds only on a full refresh or the sign-in page', () => {
		expect(bootHold('reload', '/library/45')).toBe(HOLD_MS);
		expect(bootHold('navigate', '/login')).toBe(HOLD_MS);
		expect(bootHold('navigate', '/library/45')).toBe(0);
		expect(bootHold('navigate', '/title/show/203857')).toBe(0);
		expect(bootHold(undefined, '/')).toBe(0);
	});
});
