import { reviewCount, reviewWord, timeWords } from './status-views.ts';
import { describe, expect, it } from 'vitest';
import type { DownloadRow } from '#lib/core/types.ts';

type Row = Pick<DownloadRow, 'phase' | 'state' | 'progress'>;
const r = (phase: string, state = phase): Row => ({ phase, state, progress: 0 });

describe('status words', () => {
	it('words the review count with a real space and counts only Review rows', () => {
		const rows = [r('needs_a_decision'), r('needs_a_decision'), r('failed'), r('downloading')];
		const n = reviewCount(rows);
		expect(n).toBe(2);
		expect(reviewWord(n)).toBe('2 to review');
	});
	it('counts nothing to review in an empty or calm list', () => {
		expect(reviewCount([])).toBe(0);
		expect(reviewCount([r('downloading'), r('waiting'), r('paused')])).toBe(0);
	});
	it('words the time left plainly, never "Done in"', () => {
		expect(timeWords('25m')).toBe('25m left');
		expect(timeWords('1h 5m')).toBe('1h 5m left');
		expect(timeWords('')).toBe('Nothing moving');
		expect(timeWords('25m')).not.toMatch(/Done in|ETA/);
	});
});
