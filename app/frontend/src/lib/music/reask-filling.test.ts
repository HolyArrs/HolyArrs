import { reaskWhileFilling } from './reask.ts';
import { describe, expect, it } from 'vitest';
import { isFilling } from '../core/api.ts';

const now = () => Promise.resolve();

describe('isFilling', () => {
	it('reads partial and pending', () => {
		expect(isFilling({ partial: true })).toBe(true);
		expect(isFilling({ pending: true })).toBe(true);
		expect(isFilling({ partial: false, rows: [] })).toBe(false);
		expect(isFilling(null)).toBe(false);
	});
});

describe('reaskWhileFilling', () => {
	it('asks again while partial and hands each answer on', async () => {
		const answers = [{ partial: true, n: 2 }, { partial: false, n: 3 }];
		const seen: number[] = [];
		const waits: number[] = [];
		const last = await reaskWhileFilling({ partial: true, n: 1 }, async () => answers.shift()!, (v) => seen.push(v.n), undefined, [10, 20, 30], (ms) => (waits.push(ms), now()));
		expect(last.n).toBe(3);
		expect(seen).toEqual([2, 3]);
		expect(waits).toEqual([10, 20]);
	});

	it('does not ask when the first answer is whole', async () => {
		let asked = 0;
		await reaskWhileFilling({ pending: false }, async () => (asked++, { pending: false }), () => {}, undefined, [1], now);
		expect(asked).toBe(0);
	});

	it('gives up after the pauses and keeps what is shown when an ask fails', async () => {
		let asked = 0;
		const seen: unknown[] = [];
		const last = await reaskWhileFilling({ pending: true, artists: ['a'] }, async () => {
			asked++;
			throw new Error('down');
		}, (v) => seen.push(v), undefined, [1, 1], now);
		expect(asked).toBe(2);
		expect(seen).toEqual([]);
		expect(last.artists).toEqual(['a']);
	});

	it('stops when the page is left', async () => {
		const c = new AbortController();
		let asked = 0;
		const wait = (_: number, s: AbortSignal) => (c.abort(), s.aborted ? Promise.reject(new Error('gone')) : now());
		await reaskWhileFilling({ partial: true }, async () => (asked++, { partial: true }), () => {}, c.signal, [1, 1], wait);
		expect(asked).toBe(0);
	});
});
