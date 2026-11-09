import { shortCount } from './music-pages.ts';
import { fetchRisingToday, withRising } from './music-trending.ts';
import { describe, expect, it } from 'vitest';

describe('artist popularity', () => {
	it('writes short counts', () => {
		expect(shortCount(999)).toBe('999');
		expect(shortCount(2_000_000)).toBe('2M');
		expect(shortCount(250_400_000)).toBe('250M');
	});
});

describe('rising today', () => {
	const row = (id: string, type = id) => ({ id, type });
	it('goes after the trending artists, else third, once', () => {
		const r = row('rising_today');
		expect(withRising([row('a'), row('trending_artists'), row('b')], r).map((x) => x.id)).toEqual(['a', 'trending_artists', 'rising_today', 'b']);
		expect(withRising([row('a'), row('b'), row('c')], r).map((x) => x.id)).toEqual(['a', 'b', 'rising_today', 'c']);
		expect(withRising([row('rising_today')], r)).toHaveLength(1);
		expect(withRising([row('a')], null)).toHaveLength(1);
	});
	it('is null when the server has none or fails', async () => {
		expect(await fetchRisingToday({ get: async () => { throw new Error('404'); } } as never)).toBeNull();
		expect(await fetchRisingToday({ get: async () => ({ id: 'rising_today', items: [] }) } as never)).toBeNull();
	});
});

import { askAgainOnce } from './music-pages.ts';
describe('askAgainOnce', () => {
	it('asks once more after a failure, then gives up', async () => {
		let n = 0;
		const now = async () => {};
		expect(await askAgainOnce(async () => (++n === 1 ? Promise.reject(new Error('slow')) : 'ok'), 0, now)).toBe('ok');
		n = 0;
		await expect(askAgainOnce(async () => { n++; throw new Error('down'); }, 0, now)).rejects.toThrow('down');
		expect(n).toBe(2);
	});
});
