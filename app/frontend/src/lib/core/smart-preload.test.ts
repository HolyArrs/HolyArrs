import { describe, expect, it, vi } from 'vitest';
import { createLimiter, fastEnough, idlePages, mainPicture, MAX_AT_ONCE, ownAddress } from './smart-preload.ts';

describe('fastEnough', () => {
	it('fetches ahead on 4g only, never with the data saver on', () => {
		expect(fastEnough({ effectiveType: '4g' })).toBe(true);
		expect(fastEnough({ effectiveType: '3g' })).toBe(false);
		expect(fastEnough({ effectiveType: 'slow-2g' })).toBe(false);
		expect(fastEnough({ effectiveType: '4g', saveData: true })).toBe(false);
	});
	it('takes a browser without the connection API as fast', () => {
		expect(fastEnough(null)).toBe(true);
	});
});

describe('ownAddress', () => {
	const o = 'http://192.168.1.10:18729';
	it('keeps our own paths and same-origin addresses', () => {
		expect(ownAddress('/api/v1/music/art/cover/x?size=500', o)).toBe(true);
		expect(ownAddress(`${o}/api/v1/pictures/1`, o)).toBe(true);
	});
	it('never another host (no outside calls from the browser)', () => {
		expect(ownAddress('https://image.tmdb.org/t/p/w500/a.jpg', o)).toBe(false);
		expect(ownAddress('//image.tmdb.org/a.jpg', o)).toBe(false);
		expect(ownAddress('data:image/png;base64,AA', o)).toBe(false);
		expect(ownAddress('', o)).toBe(false);
		expect(ownAddress(null, o)).toBe(false);
	});
});

describe('createLimiter', () => {
	it('runs at most 4 at once and starts the next when one ends', async () => {
		const l = createLimiter();
		const ends: (() => void)[] = [];
		const all = Array.from({ length: 7 }, () => l.run(() => new Promise<void>((r) => ends.push(r))));
		expect(l.running()).toBe(MAX_AT_ONCE);
		expect(l.waiting()).toBe(3);
		ends.shift()?.();
		await Promise.resolve();
		await Promise.resolve();
		expect(l.running()).toBe(MAX_AT_ONCE);
		expect(l.waiting()).toBe(2);
		while (ends.length || l.waiting()) {
			ends.shift()?.();
			await new Promise((r) => setTimeout(r, 0));
		}
		await Promise.all(all);
		expect(l.running()).toBe(0);
	});
});

describe('mainPicture', () => {
	it('picks our own lead picture and skips outside ones', () => {
		expect(mainPicture({ backdrop: 'https://image.tmdb.org/x.jpg', image: '/api/v1/music/art/artist/a' })).toBe('/api/v1/music/art/artist/a');
		expect(mainPicture({ artwork: ['/api/v1/art/1'] })).toBe('/api/v1/art/1');
		expect(mainPicture({ poster: 'https://elsewhere/x.jpg' })).toBeNull();
		expect(mainPicture(null)).toBeNull();
	});
});

describe('idlePages', () => {
	it('loads each page code once, our own paths only', async () => {
		vi.useFakeTimers();
		const done = new Set<string>();
		const preload = vi.fn(() => Promise.resolve());
		idlePages(['/library', '/explore', 'https://x.example/a', '/library'], preload, done);
		vi.runAllTimers();
		vi.useRealTimers();
		await Promise.resolve();
		expect(preload.mock.calls.map((c) => (c as unknown[])[0])).toEqual(['/library', '/explore']);
		idlePages(['/library'], preload, done);
		expect(preload).toHaveBeenCalledTimes(2);
	});
});
