// The artist page hero's music video: muted, looping, through HolyArrs'
// relay. It never plays for Kid accounts, with reduced motion, with data
// saver or a slow connection, or with trailers turned off; without one the
// hero keeps its photo with the slow pan.
import { mayAutoplay, type Conditions } from '../trailer.ts';

/** The answer of GET /music/artists/{mbid}?part=video. */
export interface HeroVideo {
	key: string;
	proxy_url: string;
	/** Seconds: most replayed point, else heatmap peak, else 30% in. */
	start: number;
	duration?: number;
}

/** heroVideoAllowed says whether the hero may ask for and play a video. */
export function heroVideoAllowed(c: Conditions): boolean {
	// Reduced motion and data saver win even over a device's "always".
	if (c.kid || c.reducedMotion || c.saveData) return false;
	return mayAutoplay({ ...c, phone: false });
}

/** heroVideoStart keeps the start inside the video (0 when unknown). */
export function heroVideoStart(v: Pick<HeroVideo, 'start' | 'duration'>): number {
	const s = Number.isFinite(v.start) && v.start > 0 ? v.start : 0;
	if (v.duration && s >= v.duration) return 0;
	return s;
}
