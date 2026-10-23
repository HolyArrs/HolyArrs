// The artist page hero's music video: muted, looping, through HolyArrs'
// relay. It follows exactly the trailers' rules (mayAutoplay): never for Kid
// accounts or with trailers off or "never"; with reduced motion, data saver,
// a slow connection or a phone only when this device says "always". Without
// one the hero keeps its photo with the slow pan.
import { mayAutoplay, type Conditions } from '../trailer.ts';

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

/** heroVideoStart keeps the start inside the video (0 when unknown). */
export function heroVideoStart(v: Pick<HeroVideo, 'start' | 'duration'>): number {
	const s = Number.isFinite(v.start) && v.start > 0 ? v.start : 0;
	if (v.duration && s >= v.duration) return 0;
	return s;
}
