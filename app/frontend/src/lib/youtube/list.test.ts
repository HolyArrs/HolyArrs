import { describe, expect, it } from 'vitest';
import { YT_VIEW_KEY, afterUnfollow, defaultYtSort, filterForFollow, filterYtVideos, loadYtView, noYtFilter, playlistsOf, saveYtView, showsFollow, sortYtVideos, ytFiltersOn, ytNextSort, ytStartDir, ytStateCounts, type YtSort } from './list.ts';
import type { YtFollow, YtVideo } from './view.ts';

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
	state: 'done',
	error: '',
	position_seconds: 0,
	watched_at: null,
	added_at: '2026-11-20T10:00:00Z',
	...over
});

const follow = (id: number, over: Partial<YtFollow> = {}): YtFollow => ({
	id,
	kind: 'playlist',
	yt_id: `PL${id}`,
	mirror: false,
	title: `List ${id}`,
	url: '',
	quality_max_height: 1080,
	include_shorts: false,
	keep_last: null,
	backfill: null,
	enabled: true,
	last_checked_at: null,
	last_error: '',
	...over
});

const ids = (list: YtVideo[]) => list.map((v) => v.id);

describe('sortYtVideos', () => {
	const list = [
		video(1, { title: 'banana', channel_name: 'Zed', added_at: '2026-11-01T00:00:00Z', uploaded_at: '2020-01-01T00:00:00Z', duration_seconds: 300, size_bytes: 50 }),
		video(2, { title: 'Apple', channel_name: 'alpha', added_at: '2026-11-03T00:00:00Z', uploaded_at: null, duration_seconds: 0, size_bytes: 0 }),
		video(3, { title: 'cherry', channel_name: '', added_at: '2026-11-02T00:00:00Z', uploaded_at: '2024-01-01T00:00:00Z', duration_seconds: 60, size_bytes: 900 })
	];
	const by = (key: YtSort['key'], dir: YtSort['dir']) => ids(sortYtVideos(list, { key, dir }));

	it('sorts by when a video was added, newest first by default', () => {
		expect(ids(sortYtVideos(list, defaultYtSort))).toEqual([2, 3, 1]);
		expect(by('added', 'asc')).toEqual([1, 3, 2]);
	});
	it('sorts titles and channels without minding capitals', () => {
		expect(by('title', 'asc')).toEqual([2, 1, 3]);
		expect(by('title', 'desc')).toEqual([3, 1, 2]);
		expect(by('channel', 'asc')).toEqual([2, 1, 3]);
	});
	it('puts a video with nothing to sort by last, whichever way the sort runs', () => {
		expect(by('uploaded', 'desc')).toEqual([3, 1, 2]);
		expect(by('uploaded', 'asc')).toEqual([1, 3, 2]);
		expect(by('duration', 'desc')).toEqual([1, 3, 2]);
		expect(by('duration', 'asc')).toEqual([3, 1, 2]);
		expect(by('size', 'desc')).toEqual([3, 1, 2]);
		expect(by('size', 'asc')).toEqual([1, 3, 2]);
		expect(by('channel', 'desc')).toEqual([1, 2, 3]);
	});
	it('breaks ties by the newest added and leaves the list it was given alone', () => {
		const same = [video(1, { added_at: '2026-11-01T00:00:00Z' }), video(2, { added_at: '2026-11-02T00:00:00Z' })];
		expect(ids(sortYtVideos(same, { key: 'duration', dir: 'asc' }))).toEqual([2, 1]);
		expect(ids(same)).toEqual([1, 2]);
	});
	it('starts names A to Z and everything else newest or biggest first, and flips on a second click', () => {
		expect(ytStartDir('title')).toBe('asc');
		expect(ytStartDir('channel')).toBe('asc');
		expect(ytStartDir('size')).toBe('desc');
		expect(ytNextSort(defaultYtSort, 'title')).toEqual({ key: 'title', dir: 'asc' });
		expect(ytNextSort({ key: 'title', dir: 'asc' }, 'title')).toEqual({ key: 'title', dir: 'desc' });
		expect(ytNextSort({ key: 'title', dir: 'asc' }, 'size', 'asc')).toEqual({ key: 'size', dir: 'asc' });
	});
});

describe('filterYtVideos', () => {
	const list = [
		video(1, { follow_ids: [5, 6], channel_id: 'UCa', title: 'Cats on a plane' }),
		video(2, { follow_ids: [6], channel_id: 'UCb', watched_at: '2026-11-20T11:00:00Z', state: 'done', channel_name: 'Planes Daily' }),
		video(3, { channel_id: 'UCb', state: 'failed' }),
		video(4, { follow_ids: [5], channel_id: 'UCa', state: 'queued' })
	];
	it('shows a video under each playlist that holds it', () => {
		expect(ids(filterYtVideos(list, { ...noYtFilter, playlist: 5 }))).toEqual([1, 4]);
		expect(ids(filterYtVideos(list, { ...noYtFilter, playlist: 6 }))).toEqual([1, 2]);
		expect(ids(filterYtVideos(list, noYtFilter))).toEqual([1, 2, 3, 4]);
	});
	it('narrows by channel, watched, state and every one together', () => {
		expect(ids(filterYtVideos(list, { ...noYtFilter, channel: 'UCb' }))).toEqual([2, 3]);
		expect(ids(filterYtVideos(list, { ...noYtFilter, watched: 'unwatched' }))).toEqual([1, 3, 4]);
		expect(ids(filterYtVideos(list, { ...noYtFilter, watched: 'watched' }))).toEqual([2]);
		expect(ids(filterYtVideos(list, { ...noYtFilter, state: 'failed' }))).toEqual([3]);
		expect(ids(filterYtVideos(list, { channel: 'UCa', playlist: 5, watched: 'unwatched', state: 'queued' }))).toEqual([4]);
	});
	it('finds every typed word in the title or the channel', () => {
		expect(ids(filterYtVideos(list, noYtFilter, 'PLANE'))).toEqual([1, 2]);
		expect(ids(filterYtVideos(list, noYtFilter, 'plane cats'))).toEqual([1]);
		expect(ids(filterYtVideos(list, noYtFilter, '   '))).toEqual([1, 2, 3, 4]);
		expect(ids(filterYtVideos(list, noYtFilter, 'nothing like it'))).toEqual([]);
	});
	it('counts the filters that are on and the videos in each state', () => {
		expect(ytFiltersOn(noYtFilter)).toBe(0);
		expect(ytFiltersOn({ channel: 'UCa', playlist: 5, watched: 'watched', state: 'done' })).toBe(4);
		expect(ytStateCounts(list)).toEqual({ done: 2, downloading: 0, queued: 1, failed: 1 });
	});
});

describe('follows and videos', () => {
	const follows = [follow(5, { title: 'Music' }), follow(6, { kind: 'channel', yt_id: 'UCb', title: 'Planes Daily' }), follow(7, { title: 'Talks' })];
	it('names the playlists that hold a video and leaves channels to their own column', () => {
		expect(playlistsOf(video(1, { follow_ids: [7, 6, 5] }), follows)).toEqual(['Music', 'Talks']);
		expect(playlistsOf(video(2, { follow_ids: [6] }), follows)).toEqual([]);
		expect(playlistsOf(video(3), follows)).toEqual([]);
	});
	it('shows a playlist by its follow and a channel by its channel', () => {
		expect(filterForFollow(follows[0])).toEqual({ ...noYtFilter, playlist: 5 });
		expect(filterForFollow(follows[1])).toEqual({ ...noYtFilter, channel: 'UCb' });
		expect(showsFollow({ ...noYtFilter, playlist: 5 }, follows[0])).toBe(true);
		expect(showsFollow({ ...noYtFilter, channel: 'UCb' }, follows[1])).toBe(true);
		expect(showsFollow(noYtFilter, follows[0])).toBe(false);
	});
	it('after an unfollow that deletes, drops only the videos nothing else holds', () => {
		const list = [video(1, { follow_ids: [5] }), video(2, { follow_ids: [5, 7] }), video(3, { follow_ids: [5], manual: true }), video(4, { follow_ids: [7] })];
		const out = afterUnfollow(list, 5, 'delete');
		expect(ids(out)).toEqual([2, 3, 4]);
		expect(out.map((v) => v.follow_ids)).toEqual([[7], [], [7]]);
		expect(out[2]).toBe(list[3]);
	});
	it('after an unfollow that keeps, unlinks the videos and marks them kept by hand', () => {
		const out = afterUnfollow([video(1, { follow_ids: [5] }), video(2, { follow_ids: [5, 7] })], 5, 'keep');
		expect(out.map((v) => [v.id, v.follow_ids, v.manual])).toEqual([
			[1, [], true],
			[2, [7], true]
		]);
	});
});

describe('saved view', () => {
	const store = (init: Record<string, string> = {}) => {
		const m = new Map(Object.entries(init));
		return { getItem: (k: string) => m.get(k) ?? null, setItem: (k: string, v: string) => void m.set(k, v) };
	};
	it('starts as a grid, newest added first', () => {
		expect(loadYtView(store())).toEqual({ view: 'grid', sort: defaultYtSort });
		expect(loadYtView(undefined)).toEqual({ view: 'grid', sort: defaultYtSort });
	});
	it('remembers the view and the sort', () => {
		const s = store();
		saveYtView(s, 'list', { key: 'size', dir: 'asc' });
		expect(loadYtView(s)).toEqual({ view: 'list', sort: { key: 'size', dir: 'asc' } });
	});
	it('falls back when what is stored is broken, old or refused', () => {
		expect(loadYtView(store({ [YT_VIEW_KEY]: '{not json' }))).toEqual({ view: 'grid', sort: defaultYtSort });
		expect(loadYtView(store({ [YT_VIEW_KEY]: '{"view":"wall","key":"rating","dir":"up"}' }))).toEqual({ view: 'grid', sort: defaultYtSort });
		expect(loadYtView(store({ [YT_VIEW_KEY]: '{"view":"list","key":"title"}' }))).toEqual({ view: 'list', sort: { key: 'title', dir: 'asc' } });
		const refuses = {
			getItem: () => {
				throw new Error('no storage');
			},
			setItem: () => {
				throw new Error('no storage');
			}
		};
		expect(loadYtView(refuses)).toEqual({ view: 'grid', sort: defaultYtSort });
		expect(() => saveYtView(refuses, 'list', defaultYtSort)).not.toThrow();
	});
});
