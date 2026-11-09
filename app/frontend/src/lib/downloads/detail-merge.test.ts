import { mergeDetail } from './detail-merge.ts';
import { describe, expect, it } from 'vitest';

type D = { id: number; detail?: { a: number }; files?: { n: string }[]; err?: string };

describe('mergeDetail', () => {
	it('returns the same object when nothing changed', () => {
		const prev: D = { id: 1, detail: { a: 1 }, files: [{ n: 'x' }] };
		expect(mergeDetail(prev, { id: 1, detail: { a: 1 }, files: [{ n: 'x' }] })).toBe(prev);
	});
	it('keeps unchanged fields by reference and replaces changed ones', () => {
		const prev: D = { id: 1, detail: { a: 1 }, files: [{ n: 'x' }] };
		const next = mergeDetail(prev, { id: 1, detail: { a: 2 }, files: [{ n: 'x' }] });
		expect(next).not.toBe(prev);
		expect(next.files).toBe(prev.files);
		expect(next.detail).toEqual({ a: 2 });
	});
	it('starts fresh for another download', () => {
		const prev: D = { id: 1, files: [{ n: 'x' }] };
		expect(mergeDetail(prev, { id: 2, detail: { a: 1 } })).toEqual({ id: 2, detail: { a: 1 } });
	});
	it('treats clearing an error as a change', () => {
		const prev: D = { id: 1, err: 'x' };
		expect(mergeDetail(prev, { id: 1, err: undefined }).err).toBeUndefined();
	});
});
