import { describe, expect, it } from 'vitest';
import { HOLD_MS, NAV_DELAY_MS, backoff, handoffFrom, bootHold, isUserSwitch, remaining } from './loader.ts';

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
		expect(bootHold('navigate', '/login')).toBe(0);
		expect(bootHold('reload', '/login')).toBe(0);
		expect(bootHold('navigate', '/library/45')).toBe(0);
		expect(bootHold('navigate', '/title/show/203857')).toBe(0);
		expect(bootHold(undefined, '/')).toBe(0);
	});
});

describe('sign-in hand-off timing', () => {
	it('starts the ring on the button centre at the button height', () => {
		expect(handoffFrom({ left: 500, top: 600, width: 280, height: 52 }, 1280, 800)).toEqual({ x: 0, y: 226, s: 0.5 });
	});
	it('never scales the ring above full size or to nothing', () => {
		expect(handoffFrom({ left: 0, top: 0, width: 10, height: 400 }, 100, 100).s).toBe(1);
		expect(handoffFrom({ left: 0, top: 0, width: 10, height: 2 }, 100, 100).s).toBe(0.2);
	});
});
