import { describe, expect, it } from 'vitest';
import { applyLive, backfillOf, channelsOf, clock, isWatched, progressOf, resumeAt, shouldSave, shownVideos, type YtVideo } from './view.ts';

const video = (id: number, over: Partial<YtVideo> = {}): YtVideo => ({
	id,
	follow_ids: [],
	manual: false,
	video_id: `vid${id}`,
	title: `Video ${id}`,
	channel_name: 'Channel',
	channel_id: 'UCa',
	uploaded_at: null,
	duration_seconds: 100,
	size_bytes: 0,
	state: 'queued',
	error: '',
	position_seconds: 0,
	watched_at: null,
	added_at: '2026-11-20T10:00:00Z',
	...over
});

describe('applyLive', () => {
	it('applyLive updates one card and leaves the rest', () => {
		const list = [video(1), video(2), video(3)];
		const out = applyLive(list, { id: 2, state: 'downloading', percent: 42 });
		expect(out[1]).toMatchObject({ id: 2, state: 'downloading', percent: 42 });
		expect(out[0]).toBe(list[0]);
		expect(out[2]).toBe(list[2]);
		expect(list[1].state).toBe('queued');
	});
	it('puts the error on a failed card and clears it when it moves on', () => {
		const failed = applyLive([video(1)], { id: 1, state: 'failed', percent: 0, error: 'Video unavailable' });
		expect(failed[0]).toMatchObject({ state: 'failed', error: 'Video unavailable' });
		expect(applyLive(failed, { id: 1, state: 'queued', percent: 0 })[0].error).toBe('');
	});
	it('leaves the list as it is for a video it does not hold', () => {
		const list = [video(1)];
		expect(applyLive(list, { id: 9, state: 'done', percent: 100 })).toBe(list);
	});
});

describe('watching', () => {
	it('isWatched at 90 percent', () => {
		expect(isWatched(89, 100)).toBe(false);
		expect(isWatched(90, 100)).toBe(true);
		expect(isWatched(5, 0)).toBe(false);
	});
	it('progressOf stays between 0 and 1', () => {
		expect(progressOf({ position_seconds: 25, duration_seconds: 100 })).toBe(0.25);
		expect(progressOf({ position_seconds: 250, duration_seconds: 100 })).toBe(1);
		expect(progressOf({ position_seconds: 10, duration_seconds: 0 })).toBe(0);
		expect(progressOf({ position_seconds: -5, duration_seconds: 100 })).toBe(0);
	});
	it('resumeAt starts watched videos from the start', () => {
		expect(resumeAt({ position_seconds: 40, duration_seconds: 100, watched_at: null })).toBe(40);
		expect(resumeAt({ position_seconds: 95, duration_seconds: 100, watched_at: '2026-11-20T11:00:00Z' })).toBe(0);
		expect(resumeAt({ position_seconds: 95, duration_seconds: 100, watched_at: null })).toBe(0);
		expect(resumeAt({ position_seconds: -3, duration_seconds: 100, watched_at: null })).toBe(0);
	});
	it('shouldSave every ten seconds', () => {
		expect(shouldSave(0, 9_999)).toBe(false);
		expect(shouldSave(0, 10_000)).toBe(true);
	});
});

describe('backfillOf', () => {
	it('turns the channel choice into the backfill to send', () => {
		expect(backfillOf('newest20', 7)).toBe(20);
		expect(backfillOf('newest', 7)).toBe(7);
		expect(backfillOf('newest', 0)).toBe(1);
		expect(backfillOf('newest', 3.6)).toBe(4);
		expect(backfillOf('all', 7)).toBeGreaterThanOrEqual(100_000);
	});
});

describe('card words', () => {
	it('clock shows minutes and seconds, and hours past an hour', () => {
		expect(clock(872)).toBe('14:32');
		expect(clock(59)).toBe('0:59');
		expect(clock(3723)).toBe('1:02:03');
		expect(clock(0)).toBe('');
	});
	it('channelsOf lists each channel once, by name', () => {
		const list = [video(1, { channel_id: 'UCb', channel_name: 'Zed' }), video(2, { channel_id: 'UCa', channel_name: 'Alpha' }), video(3, { channel_id: 'UCb', channel_name: 'Zed' }), video(4, { channel_id: '' })];
		expect(channelsOf(list)).toEqual([
			{ id: 'UCa', name: 'Alpha' },
			{ id: 'UCb', name: 'Zed' }
		]);
	});
	it('shownVideos leaves out skipped and removed ones, newest first', () => {
		const list = [
			video(1, { added_at: '2026-11-19T10:00:00Z' }),
			video(2, { added_at: '2026-11-21T10:00:00Z', state: 'skipped' }),
			video(3, { added_at: '2026-11-21T10:00:00Z' }),
			video(4, { added_at: '2026-11-20T10:00:00Z', state: 'removed' }),
			video(5, { added_at: '2026-11-21T10:00:00Z', state: 'failed' })
		];
		expect(shownVideos(list).map((v) => v.id)).toEqual([5, 3, 1]);
	});
	it('shownVideos puts the newest upload first among videos added together', () => {
		const at = '2026-11-21T10:00:00Z';
		const list = [video(1, { added_at: at, uploaded_at: '2026-11-01T00:00:00Z' }), video(2, { added_at: at, uploaded_at: '2026-10-01T00:00:00Z' }), video(3, { added_at: at, uploaded_at: null })];
		expect(shownVideos(list).map((v) => v.id)).toEqual([1, 2, 3]);
	});
});
