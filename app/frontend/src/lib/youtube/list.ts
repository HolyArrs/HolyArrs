// The YouTube page's sort, filter and view rules, shaped like the Library's: pure, so they are tested without a browser.
import type { YtFollow, YtState, YtVideo } from './view.ts';

export type YtView = 'grid' | 'list';
export type YtSortKey = 'added' | 'uploaded' | 'title' | 'channel' | 'duration' | 'size';
export type YtSortDir = 'asc' | 'desc';
export interface YtSort {
	key: YtSortKey;
	dir: YtSortDir;
}
export type YtStateFilter = 'any' | Extract<YtState, 'done' | 'queued' | 'downloading' | 'failed'>;

/** What narrows the list. `channel` is a channel id, `playlist` a follow id; '' and 0 do not filter. */
export interface YtFilter {
	channel: string;
	playlist: number;
	watched: 'any' | 'unwatched' | 'watched';
	state: YtStateFilter;
}

export const ytSorts: YtSortKey[] = ['added', 'uploaded', 'title', 'channel', 'duration', 'size'];
export const ytStates: Exclude<YtStateFilter, 'any'>[] = ['done', 'downloading', 'queued', 'failed'];
export const noYtFilter: YtFilter = { channel: '', playlist: 0, watched: 'any', state: 'any' };
export const defaultYtSort: YtSort = { key: 'added', dir: 'desc' };

/** The direction a sort starts in: names A to Z, everything else biggest or newest first. */
export function ytStartDir(key: YtSortKey): YtSortDir {
	return key === 'title' || key === 'channel' ? 'asc' : 'desc';
}

/** The sort after a click on a key: a new key starts in its own direction, the same key flips. */
export function ytNextSort(now: YtSort, key: YtSortKey, dir?: YtSortDir): YtSort {
	if (dir) return { key, dir };
	return now.key === key ? { key, dir: now.dir === 'asc' ? 'desc' : 'asc' } : { key, dir: ytStartDir(key) };
}

const time = (s: string | null): number | null => {
	const n = s ? Date.parse(s) : NaN;
	return Number.isNaN(n) ? null : n;
};

/** The videos in the chosen order. A video with nothing to sort by (no upload date, no length, no size) goes last whichever way the sort runs; ties keep the newest added first. */
export function sortYtVideos(list: YtVideo[], sort: YtSort): YtVideo[] {
	const flip = sort.dir === 'asc' ? 1 : -1;
	const value = (v: YtVideo): number | string | null => {
		switch (sort.key) {
			case 'added':
				return time(v.added_at);
			case 'uploaded':
				return time(v.uploaded_at);
			case 'title':
				return (v.title || v.video_id).toLocaleLowerCase();
			case 'channel':
				return v.channel_name ? v.channel_name.toLocaleLowerCase() : null;
			case 'duration':
				return v.duration_seconds > 0 ? v.duration_seconds : null;
			case 'size':
				return v.size_bytes > 0 ? v.size_bytes : null;
		}
	};
	const tie = (a: YtVideo, b: YtVideo) => (time(b.added_at) ?? 0) - (time(a.added_at) ?? 0) || (time(b.uploaded_at) ?? 0) - (time(a.uploaded_at) ?? 0) || b.id - a.id;
	return list.slice().sort((a, b) => {
		const x = value(a);
		const y = value(b);
		if (x === null || y === null) return x === y ? tie(a, b) : x === null ? 1 : -1;
		const c = typeof x === 'string' ? x.localeCompare(y as string) : x - (y as number);
		return c ? c * flip : tie(a, b);
	});
}

/** The videos that pass the filter and hold every typed word in their title or channel. A video shows under each playlist that holds it. */
export function filterYtVideos(list: YtVideo[], f: YtFilter, q = ''): YtVideo[] {
	const words = q.toLocaleLowerCase().split(/\s+/).filter(Boolean);
	return list.filter((v) => {
		if (f.channel && v.channel_id !== f.channel) return false;
		if (f.playlist && !v.follow_ids.includes(f.playlist)) return false;
		if (f.watched === 'unwatched' && v.watched_at) return false;
		if (f.watched === 'watched' && !v.watched_at) return false;
		if (f.state !== 'any' && v.state !== f.state) return false;
		if (!words.length) return true;
		const hay = `${v.title} ${v.channel_name}`.toLocaleLowerCase();
		return words.every((w) => hay.includes(w));
	});
}

/** How many filters are on, for the Sort & filter button. */
export function ytFiltersOn(f: YtFilter): number {
	return Number(!!f.channel) + Number(!!f.playlist) + Number(f.watched !== 'any') + Number(f.state !== 'any');
}

/** How many of the shown videos stand in each state, for the filter chips. */
export function ytStateCounts(list: YtVideo[]): Record<Exclude<YtStateFilter, 'any'>, number> {
	const out = { done: 0, downloading: 0, queued: 0, failed: 0 };
	for (const v of list) if (v.state in out) out[v.state as keyof typeof out]++;
	return out;
}

/** The names of the playlists that hold a video, in the order the person followed them. Channels are left out: the channel has its own column. */
export function playlistsOf(v: Pick<YtVideo, 'follow_ids'>, follows: YtFollow[]): string[] {
	return follows.filter((f) => f.kind === 'playlist' && v.follow_ids.includes(f.id)).map((f) => f.title);
}

/** The filter that shows one follow's videos: a playlist by its follow, a channel by its channel. */
export function filterForFollow(f: Pick<YtFollow, 'id' | 'kind' | 'yt_id'>): YtFilter {
	return f.kind === 'playlist' ? { ...noYtFilter, playlist: f.id } : { ...noYtFilter, channel: f.yt_id };
}

/** Whether the filter shows exactly one follow's videos (for the pressed name in the Following row). */
export function showsFollow(filter: YtFilter, f: Pick<YtFollow, 'id' | 'kind' | 'yt_id'>): boolean {
	return f.kind === 'playlist' ? filter.playlist === f.id : filter.channel === f.yt_id;
}

/** The list after an unfollow, before the server's own list comes in. Keeping the videos only unlinks them; deleting them drops the ones nothing else holds. */
export function afterUnfollow(list: YtVideo[], followId: number, files: 'keep' | 'delete'): YtVideo[] {
	const out: YtVideo[] = [];
	for (const v of list) {
		if (!v.follow_ids.includes(followId)) {
			out.push(v);
			continue;
		}
		const rest = v.follow_ids.filter((id) => id !== followId);
		if (files === 'delete' && rest.length === 0 && !v.manual) continue;
		out.push({ ...v, follow_ids: rest, manual: v.manual || files === 'keep' });
	}
	return out;
}

export const YT_VIEW_KEY = 'holyarrs.youtube.view';

/** Reads the saved view and sort; storage may be missing, refuse or hold something old. */
export function loadYtView(storage: Pick<Storage, 'getItem'> | undefined): { view: YtView; sort: YtSort } {
	const fallback = { view: 'grid' as YtView, sort: defaultYtSort };
	try {
		const raw = JSON.parse(storage?.getItem(YT_VIEW_KEY) ?? 'null') as { view?: unknown; key?: unknown; dir?: unknown } | null;
		if (!raw || typeof raw !== 'object') return fallback;
		const view = raw.view === 'list' ? 'list' : 'grid';
		const key = ytSorts.includes(raw.key as YtSortKey) ? (raw.key as YtSortKey) : defaultYtSort.key;
		const dir = raw.dir === 'asc' || raw.dir === 'desc' ? raw.dir : ytStartDir(key);
		return { view, sort: { key, dir } };
	} catch {
		return fallback;
	}
}

/** Saves the view and sort; a refusal only means they last this visit. */
export function saveYtView(storage: Pick<Storage, 'setItem'> | undefined, view: YtView, sort: YtSort): void {
	try {
		storage?.setItem(YT_VIEW_KEY, JSON.stringify({ view, key: sort.key, dir: sort.dir }));
	} catch {
		// Private windows may refuse storage.
	}
}
