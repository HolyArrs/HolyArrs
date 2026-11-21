// The YouTube routes, for the person's own videos and follows.
import { api } from '#lib/core/api.ts';
import { to } from '#lib/core/paths.ts';
import type { YtFollow, YtProbe, YtSettings, YtStatus, YtVideo } from './view.ts';

export interface NewFollow {
	url: string;
	backfill?: number;
	keep_last?: number | null;
	include_shorts?: boolean;
	mirror?: boolean;
}

export interface FollowEdit {
	quality_max_height?: number;
	enabled?: boolean;
	keep_last?: number | null;
	include_shorts?: boolean;
	mirror?: boolean;
}

export const ytStatus = () => api.get<YtStatus>('/youtube/status');
export const ytVideos = () => api.get<YtVideo[]>('/youtube/videos');
export const ytVideo = (id: number) => api.get<YtVideo>(`/youtube/videos/${id}`);
export const ytFollows = () => api.get<YtFollow[]>('/youtube/follows');
export const ytProbe = (url: string) => api.post<YtProbe>('/youtube/probe', { url });
export const ytAddVideo = (url: string) => api.post<YtVideo>('/youtube/videos', { url });
export const ytAddFollow = (f: NewFollow) => api.post<YtFollow>('/youtube/follows', f);
export const ytEditFollow = (id: number, edit: FollowEdit) => api.put<YtFollow>(`/youtube/follows/${id}`, edit);
export const ytUnfollow = (id: number, files: 'keep' | 'delete') => api.del(`/youtube/follows/${id}?files=${files}`);
export const ytRemove = (id: number) => api.del(`/youtube/videos/${id}`);
/** Saves where the person stopped; keepalive lets the save finish when the page is closed right after. */
export const ytSavePosition = (id: number, seconds: number, duration: number) =>
	api.request<{ watched: boolean }>(`/youtube/videos/${id}/position`, { method: 'PUT', body: { seconds, duration }, keepalive: true });
export const ytSettings = () => api.get<YtSettings>('/settings/youtube');
export const ytSaveSettings = (folder: string, minFreeGB: number) => api.put<YtSettings>('/settings/youtube', { folder, min_free_gb: minFreeGB });
export const ytRetry = (id: number) => api.post<YtVideo>(`/youtube/videos/${id}/retry`);

/** The thumbnail of a downloaded video. */
export const ytThumb = (id: number) => to(`/api/v1/youtube/videos/${id}/thumb`);
/** The file of a downloaded video, to play. */
export const ytFile = (id: number) => to(`/api/v1/youtube/videos/${id}/file`);
/** The file of a downloaded video, saved to the device under its own name. */
export const ytSaveLink = (id: number) => to(`/api/v1/youtube/videos/${id}/file?download=1`);
