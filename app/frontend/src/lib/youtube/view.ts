// The YouTube page's data and the pure rules behind it: live updates, filters and how far a video was watched.

export type YtState = 'queued' | 'downloading' | 'done' | 'failed' | 'skipped' | 'removed';

/** One of the person's videos, as GET /youtube/videos answers it. `percent` comes only from live events. */
export interface YtVideo {
	id: number;
	/** The follows that hold the video; a video can be in several playlists and channels. */
	follow_ids: number[];
	/** The person added it by its own link, so no follow deletes it. */
	manual: boolean;
	video_id: string;
	title: string;
	channel_name: string;
	channel_id: string;
	uploaded_at: string | null;
	duration_seconds: number;
	size_bytes: number;
	state: YtState;
	error: string;
	position_seconds: number;
	watched_at: string | null;
	added_at: string;
	percent?: number;
}

/** A playlist or channel the person follows. `keep_last` null keeps every video; `backfill` null took every video. `mirror` (playlists only) also removes what leaves the playlist. */
export interface YtFollow {
	id: number;
	kind: 'playlist' | 'channel';
	yt_id: string;
	mirror: boolean;
	title: string;
	url: string;
	quality_max_height: number;
	include_shorts: boolean;
	keep_last: number | null;
	backfill: number | null;
	enabled: boolean;
	last_checked_at: string | null;
	last_error: string;
}

/** A live event on the youtube topic: one video moved on. */
export interface YtLive {
	id: number;
	state: YtState;
	percent: number;
	error?: string;
}

/** What a pasted link points at, for the confirm card. `count` is 0 when not known (channels). */
export interface YtProbe {
	kind: 'video' | 'playlist' | 'channel';
	id: string;
	url: string;
	title: string;
	channel_name: string;
	channel_id: string;
	duration_seconds: number;
	count: number;
	live_status: string;
}

/** The programs, the folder and free space, from GET /youtube/status. */
export interface YtStatus {
	missing: string[];
	folder_set: boolean;
	low_space: boolean;
	free_bytes: number;
	floor_bytes: number;
}

/** Settings > YouTube: the folder, the free space to keep, the programs and the totals over everyone (never titles). */
export interface YtSettings {
	folder: string;
	min_free_gb: number;
	ytdlp_updated_at: string;
	ytdlp_update_error: string;
	totals: { videos: number; bytes: number; people: number };
	ytdlp_path: string;
	ffmpeg_path: string;
	version: string;
}

/** The channel choice on the confirm card: the newest 20, the newest N, or every video. */
export type BackfillChoice = 'newest20' | 'newest' | 'all';

/** Sent as the backfill for "All": a channel with no backfill takes the newest 20, so "All" is a number no channel reaches. */
export const ALL_VIDEOS = 1_000_000;

export const SAVE_EVERY_MS = 10_000;

/** The list with the one video the event names moved on; the same list when it holds no such video. */
export function applyLive(list: YtVideo[], ev: { id: number; state: YtState; percent: number; error?: string }): YtVideo[] {
	const at = list.findIndex((v) => v.id === ev.id);
	if (at < 0) return list;
	const out = list.slice();
	out[at] = { ...list[at], state: ev.state, percent: ev.percent, error: ev.error ?? '' };
	return out;
}

/** How far into the video the person is, 0 to 1. */
export function progressOf(v: Pick<YtVideo, 'position_seconds' | 'duration_seconds'>): number {
	if (!(v.duration_seconds > 0) || !(v.position_seconds > 0)) return 0;
	return Math.min(1, v.position_seconds / v.duration_seconds);
}

/** Past 90 percent counts as watched. */
export function isWatched(seconds: number, duration: number): boolean {
	return duration > 0 && seconds * 10 >= duration * 9;
}

/** The position is saved at most every ten seconds while playing. */
export function shouldSave(lastSavedAt: number, now: number): boolean {
	return now - lastSavedAt >= SAVE_EVERY_MS;
}

/** Where playing starts: where the person stopped, or the start for a watched video. */
export function resumeAt(v: Pick<YtVideo, 'position_seconds' | 'duration_seconds' | 'watched_at'>): number {
	if (v.watched_at || isWatched(v.position_seconds, v.duration_seconds)) return 0;
	return Math.max(0, v.position_seconds);
}

/** The backfill to send for a channel choice; "newest" takes at least one. */
export function backfillOf(choice: BackfillChoice, n: number): number {
	if (choice === 'newest20') return 20;
	if (choice === 'all') return ALL_VIDEOS;
	return Math.max(1, Math.round(Number.isFinite(n) ? n : 1));
}

/** A video length as a clock: 14:32, or 1:02:03 past an hour. Empty when not known. */
export function clock(seconds: number): string {
	if (!(seconds > 0)) return '';
	const s = Math.round(seconds);
	const h = Math.floor(s / 3600);
	const m = Math.floor((s % 3600) / 60);
	const ss = String(s % 60).padStart(2, '0');
	return h > 0 ? `${h}:${String(m).padStart(2, '0')}:${ss}` : `${m}:${ss}`;
}

/** The channels among the videos, once each, by name. */
export function channelsOf(list: YtVideo[]): { id: string; name: string }[] {
	const seen = new Map<string, string>();
	for (const v of list) if (v.channel_id && !seen.has(v.channel_id)) seen.set(v.channel_id, v.channel_name || v.channel_id);
	return [...seen].map(([id, name]) => ({ id, name })).sort((a, b) => a.name.localeCompare(b.name));
}

/** The videos the grid shows: never the skipped or removed ones. Newest added first; one check adds many at once, so those go newest upload first. */
export function shownVideos(list: YtVideo[]): YtVideo[] {
	return list
		.filter((v) => v.state !== 'skipped' && v.state !== 'removed')
		.sort((a, b) => later(a.added_at, b.added_at) || later(a.uploaded_at, b.uploaded_at) || b.id - a.id);
}

// Sorts the later time first; a missing or unreadable time counts as the earliest.
function later(a: string | null, b: string | null): number {
	const ta = a ? Date.parse(a) : NaN;
	const tb = b ? Date.parse(b) : NaN;
	return (Number.isNaN(tb) ? -Infinity : tb) - (Number.isNaN(ta) ? -Infinity : ta) || 0;
}

/** Saves where the person is: every ten seconds while playing, on pause and on leaving. A second already saved last is not sent again. */
export function makeSaver(
	send: (s: number, d: number) => void,
	now = () => Date.now()
): { tick(s: number, d: number): void; pause(s: number, d: number): void; leave(s: number, d: number): void } {
	let lastAt = now();
	let lastSecond = -1;
	const save = (s: number, d: number) => {
		if (!(d > 0) || !(s >= 0)) return;
		const second = Math.floor(s);
		if (second === lastSecond) return;
		lastSecond = second;
		lastAt = now();
		send(second, Math.round(d));
	};
	return {
		tick: (s, d) => {
			if (shouldSave(lastAt, now())) save(s, d);
		},
		pause: save,
		leave: save
	};
}
