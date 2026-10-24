// Pass-through playback: HolyArrs' relayed HLS stream on a <video>. Native
// HLS (Safari) plays it directly; elsewhere hls.js feeds it. hls.js ships
// as its own file (scripts/vendor-hls.mjs copies the pinned version) and is
// loaded only when first needed. A fatal hls.js error becomes the video's
// error event, so players treat both the same way.
import type HlsType from 'hls.js';
import { to } from './paths.ts';

/** The pinned hls.js (package.json); a test checks they agree. */
export const HLS_VERSION = '1.7.3';
export const hlsScript = `/vendor/hls-${HLS_VERSION}.min.js`;

let loading: Promise<typeof HlsType> | null = null;
function loadHls(): Promise<typeof HlsType> {
	loading ??= (import(/* @vite-ignore */ to(hlsScript)) as Promise<{ default: typeof HlsType }>).then((m) => m.default);
	loading.catch(() => (loading = null));
	return loading;
}

/** attachHLS plays url on video from start seconds; the answer detaches.
 * target is the tallest picture worth fetching (playerTarget; hover cards keep 720). */
export async function attachHLS(video: HTMLVideoElement, url: string, native: boolean, start = 0, target = 720): Promise<() => void> {
	if (native) {
		if (start > 0) video.addEventListener('loadedmetadata', () => (video.currentTime = start), { once: true });
		video.src = url;
		return () => {
			video.removeAttribute('src');
			video.load();
		};
	}
	const Hls = await loadHls();
	if (!Hls.isSupported()) throw new Error('no MediaSource');
	const hls = new Hls({ startPosition: start, maxBufferLength: 20, capLevelToPlayerSize: false, enableWorker: true });
	// YouTube's stream mixes H.264, VP9 and AV1 at the same sizes; switching
	// codec mid-play stalls the picture. Keep H.264 only, start at the tallest
	// that fits the player and let ABR move only up to that.
	hls.on(Hls.Events.MANIFEST_PARSED, () => {
		const { drop, start: at } = planLevels(hls.levels.map((l) => ({ codec: l.videoCodec ?? l.codecSet ?? '', height: l.height })), target);
		for (const i of drop) hls.removeLevel(i);
		if (at >= 0) {
			hls.autoLevelCapping = at;
			hls.startLevel = hls.nextLevel = at;
		}
	});
	// It also carries auto-dubbed audio next to the original: keep the original.
	hls.on(Hls.Events.AUDIO_TRACKS_UPDATED, () => {
		const i = originalAudio(hls.audioTracks.map((t) => ({ name: t.name ?? '', isDefault: !!t.default })));
		if (i >= 0 && hls.audioTrack !== i) hls.audioTrack = i;
	});
	hls.on(Hls.Events.ERROR, (_e, d) => {
		if (d.fatal) {
			video.dataset.hlsError = `${d.type}/${d.details}${d.response?.code ? ' ' + d.response.code : ''}`;
			video.dispatchEvent(new Event('error'));
		}
	});
	if (typeof location !== 'undefined' && location.search.includes('trailerdebug')) (window as unknown as { __hls?: unknown[] }).__hls = [...((window as unknown as { __hls?: unknown[] }).__hls ?? []), hls];
	hls.loadSource(url);
	hls.attachMedia(video);
	return () => hls.destroy();
}

/** playerTarget is the picture height a player of w×h css pixels needs at
 * the pixel ratio: cover fills the box (a hero), contain fits in it (full
 * screen). Capped at 1080: never 4K. */
export function playerTarget(w: number, h: number, fit: 'cover' | 'contain', ratio = globalThis.devicePixelRatio || 1): number {
	const tall = fit === 'cover' ? Math.max(h, (w * 9) / 16) : Math.min(h, (w * 9) / 16);
	return Math.min(1080, Math.max(360, Math.ceil(tall * ratio)));
}

/** screenTarget sizes a player that can go full screen by the whole screen,
 * not the window: a small window on a big or ultrawide screen would
 * otherwise lock the stream to a low level that full screen keeps. */
export function screenTarget(fit: 'cover' | 'contain', ratio = globalThis.devicePixelRatio || 1): number {
	const s = typeof screen === 'undefined' ? null : screen;
	const w = Math.max(s?.width ?? 0, globalThis.innerWidth || 0);
	const h = Math.max(s?.height ?? 0, globalThis.innerHeight || 0);
	return playerTarget(w || 1280, h || 720, fit, ratio);
}

/** planLevels keeps H.264 levels only (when there are any): the indexes to
 * drop, highest first, and the start level among those left (the shortest
 * that covers target, up to 1080p), which is also the ABR cap. */
export function planLevels(levels: { codec: string; height: number }[], target = 720): { drop: number[]; start: number } {
	target = Math.min(target, 1080);
	const avc = (c: string) => c.toLowerCase().startsWith('avc1');
	const keep = levels.map((l, i) => ({ ...l, i })).filter((l) => avc(l.codec));
	if (!keep.length) return { drop: [], start: -1 };
	const drop = levels.map((l, i) => (avc(l.codec) ? -1 : i)).filter((i) => i >= 0).reverse();
	// The shortest level that still covers the target (never above 1080p),
	// else the tallest under it, else the shortest.
	const fits = keep.map((l, k) => ({ h: l.height, k })).filter((l) => l.h <= 1080);
	const cover = fits.filter((l) => l.h >= target).sort((a, b) => a.h - b.h)[0];
	const under = fits.sort((a, b) => b.h - a.h)[0];
	return { drop, start: (cover ?? under)?.k ?? 0 };
}

/** originalAudio picks the track that is not an auto-dub: the default one, else the first such. */
export function originalAudio(tracks: { name: string; isDefault: boolean }[]): number {
	const real = (t: { name: string }) => !/dubbed|auto-?dub/i.test(t.name);
	const d = tracks.findIndex((t) => t.isDefault && real(t));
	return d >= 0 ? d : tracks.findIndex(real);
}
