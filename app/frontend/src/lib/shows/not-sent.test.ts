import { describe, expect, it } from 'vitest';
import { createNotSentWatch } from './not-sent.ts';

describe('createNotSentWatch', () => {
	it('names the searched episode and the reason once', () => {
		const w = createNotSentWatch();
		w.watch([7], 'S01E02');
		expect(w.take({ id: 7, state: 'wanted', reason: 'not sent: no torrent Downloader is set up', not_sent: true })).toEqual({ name: 'S01E02', reason: 'no torrent Downloader is set up' });
		expect(w.take({ id: 7, not_sent: true, reason: 'x' })).toBeNull();
	});
	it('ignores other Slots and ordinary slot events', () => {
		const w = createNotSentWatch();
		w.watch([7], 'S01E02');
		expect(w.take({ id: 8, not_sent: true, reason: 'x' })).toBeNull();
		expect(w.take({ id: 7, state: 'downloading' })).toBeNull();
		expect(w.take(null)).toBeNull();
		expect(w.take({ id: 7, not_sent: true, reason: 'x' })?.name).toBe('S01E02');
	});
	it('forgets old searches', () => {
		let t = 0;
		const w = createNotSentWatch(() => t);
		w.watch([7], 'S01E02');
		t = 31 * 60 * 1000;
		expect(w.take({ id: 7, not_sent: true, reason: 'x' })).toBeNull();
	});
});
