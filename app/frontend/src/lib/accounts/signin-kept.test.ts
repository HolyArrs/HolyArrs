import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { setBase } from '#lib/core/paths.ts';
import { local, ownShots, readKept, writeCache } from './signin-kept.ts';

const CACHE = 'holyarrs-signin-art';
const img = (url: string) => ({ url, w: 1920, h: 1080 });

function fakeStorage() {
	const m = new Map<string, string>();
	return {
		getItem: (k: string) => m.get(k) ?? null,
		setItem: (k: string, v: string) => void m.set(k, String(v)),
		removeItem: (k: string) => void m.delete(k)
	};
}

describe('sign-in kept art', () => {
	beforeEach(() => {
		vi.stubGlobal('localStorage', fakeStorage());
		setBase('');
	});
	afterEach(() => {
		vi.unstubAllGlobals();
		setBase('');
	});

	it('local accepts only same-origin paths', () => {
		expect(local('/art/a.jpg')).toBe(true);
		expect(local('//evil/x')).toBe(false);
		expect(local('https://image.tmdb.org/a')).toBe(false);
		expect(local('art/a')).toBe(false);
		expect(local('')).toBe(false);
		expect(local(undefined)).toBe(false);
		expect(local(5)).toBe(false);
	});

	it('ownShots keeps only same-origin art', () => {
		const s = ownShots(
			[
				{ backdrop: img('https://image.tmdb.org/a') },
				{ backdrop: img('//evil/b') },
				{ backdrop: img('/art/b'), poster: img('//x'), logo: '/l' },
				{ backdrop: img('/art/c'), poster: img('/art/p'), logo: 'https://x/l' },
				null
			],
			false
		);
		expect(s).toHaveLength(2);
		expect(s[0].backdrop.url).toBe('/art/b');
		expect(s[0].poster).toBeUndefined();
		expect(s[0].logo).toBe('/l');
		expect(s[1].poster?.url).toBe('/art/p');
		expect(s[1].logo).toBe('');
	});

	it('ownShots ignores anything that is not a list', () => {
		expect(ownShots(undefined, false)).toEqual([]);
		expect(ownShots({ backdrop: img('/a') }, false)).toEqual([]);
	});

	it('ownShots puts the app base path in front only when asked', () => {
		setBase('/arrs/');
		const list = [{ backdrop: img('/art/b'), poster: img('/art/p'), logo: '/l' }];
		const off = ownShots(list, false)[0];
		expect(off.backdrop.url).toBe('/art/b');
		const on = ownShots(list, true)[0];
		expect(on.backdrop.url).toBe('/arrs/art/b');
		expect(on.poster?.url).toBe('/arrs/art/p');
		expect(on.logo).toBe('/arrs/l');
	});

	it('readKept survives an empty or broken cache', () => {
		expect(readKept()).toEqual({ shots: [], first: undefined, thumb: null });
		localStorage.setItem(CACHE, '{bad');
		expect(readKept()).toEqual({ shots: [], thumb: null });
		localStorage.setItem(CACHE, 'null');
		expect(readKept().thumb).toBeNull();
	});

	it('readKept needs a jpeg thumb at a same-origin address', () => {
		const t = (url: string, t64: string) => JSON.stringify({ thumb: { url, t24: 'x', t64, pos: '50% 50%' } });
		localStorage.setItem(CACHE, t('/a', 'data:image/png;base64,x'));
		expect(readKept().thumb).toBeNull();
		localStorage.setItem(CACHE, t('//evil/a', 'data:image/jpeg;base64,x'));
		expect(readKept().thumb).toBeNull();
		localStorage.setItem(CACHE, t('https://image.tmdb.org/a', 'data:image/jpeg;base64,x'));
		expect(readKept().thumb).toBeNull();
		localStorage.setItem(CACHE, t('/a', 'data:image/jpeg;base64,x'));
		expect(readKept().thumb?.url).toBe('/a');
	});

	it('writeCache keeps the thumb only for the first picture, and reads back filtered', () => {
		const thumb = { url: '/a', t24: 'x', t64: 'data:image/jpeg;base64,x', pos: '0 0' };
		const shots = [{ backdrop: img('/a'), logo: '' }, { backdrop: img('https://tmdb/x'), logo: '' }];
		writeCache(shots, '/a', thumb);
		const k = readKept();
		expect(k.first).toBe('/a');
		expect(k.thumb?.url).toBe('/a');
		expect(k.shots).toHaveLength(1);
		writeCache(shots, '/other', thumb);
		expect(readKept().thumb).toBeNull();
	});

	it('writeCache does not throw when storage is full', () => {
		vi.stubGlobal('localStorage', {
			getItem: () => null,
			setItem: () => {
				throw new Error('quota');
			}
		});
		expect(() => writeCache([], '', null)).not.toThrow();
	});
});
