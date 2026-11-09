// Rules and helpers for the artist hero's muted looping music video.
import type { TrailerSource } from './autoplay.svelte.ts';

export interface HeroVideo {
	key: string;
	proxy_url: string;
	start: number;
	duration?: number;
	expires_at?: string;
}

// Relay first: label videos often refuse the embedded player.
export function pickedSources(key: string, v: Pick<HeroVideo, 'key' | 'proxy_url'> | null): TrailerSource[] {
	const out: TrailerSource[] = [];
	if (v?.proxy_url && v.key === key) out.push({ kind: 'pass', url: v.proxy_url, key });
	out.push({ kind: 'embed', key });
	return out;
}

// iPhone Safari cannot full screen a page element, so it falls back to the video's own player.
export function heroFullscreen(enabled: boolean | undefined, box: { requestFullscreen?: unknown } | undefined, video: { webkitEnterFullscreen?: unknown } | undefined): 'page' | 'video' | null {
	if (enabled && typeof box?.requestFullscreen === 'function') return 'page';
	if (typeof video?.webkitEnterFullscreen === 'function') return 'video';
	return null;
}

// A start at or past the end would show nothing, so it restarts from 0.
export function heroVideoStart(v: Pick<HeroVideo, 'start' | 'duration'>): number {
	const s = Number.isFinite(v.start) && v.start > 0 ? v.start : 0;
	if (v.duration && s >= v.duration) return 0;
	return s;
}
