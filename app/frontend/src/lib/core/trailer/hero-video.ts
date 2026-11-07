// The artist page hero's music video: muted, looping, through HolyArrs'
// relay. It follows exactly the trailers' rules (mayAutoplay): never for Kid
// accounts or with trailers off or "never"; with reduced motion, data saver,
// a slow connection or a phone only when this device says "always". Without
// one the hero keeps its photo with the slow pan.
import { mayAutoplay, type Conditions } from './trailer.ts';
import type { TrailerSource } from './autoplay.svelte.ts';

/** The answer of GET /music/artists/{mbid}?part=video. */
export interface HeroVideo {
	key: string;
	proxy_url: string;
	/** Seconds: most replayed point, else heatmap peak, else 30% in. */
	start: number;
	duration?: number;
	/** When the relayed stream stops working. */
	expires_at?: string;
}

/** heroVideoAllowed says whether the hero may ask for and play a video. */
export function heroVideoAllowed(c: Conditions): boolean {
	return mayAutoplay(c);
}

/**
 * pickedSources orders the sources for a video picked in the Music videos
 * row (v: the answer of ?part=video&id=<key>): the relayed stream first,
 * since label videos refuse YouTube's embedded player (error 150), then the
 * embed.
 */
export function pickedSources(key: string, v: Pick<HeroVideo, 'key' | 'proxy_url'> | null): TrailerSource[] {
	const out: TrailerSource[] = [];
	if (v?.proxy_url && v.key === key) out.push({ kind: 'pass', url: v.proxy_url, key });
	out.push({ kind: 'embed', key });
	return out;
}

/**
 * heroFullscreen picks how the hero's video goes full screen: the browser's
 * own full screen on the hero (the video fit whole, with its own exit, pause
 * and sound), or where a page may not (iPhone Safari) the video's player.
 */
export function heroFullscreen(enabled: boolean | undefined, box: { requestFullscreen?: unknown } | undefined, video: { webkitEnterFullscreen?: unknown } | undefined): 'page' | 'video' | null {
	if (enabled && typeof box?.requestFullscreen === 'function') return 'page';
	if (typeof video?.webkitEnterFullscreen === 'function') return 'video';
	return null;
}

/** heroVideoStart keeps the start inside the video (0 when unknown). */
export function heroVideoStart(v: Pick<HeroVideo, 'start' | 'duration'>): number {
	const s = Number.isFinite(v.start) && v.start > 0 ? v.start : 0;
	if (v.duration && s >= v.duration) return 0;
	return s;
}
