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

/** attachHLS plays url on video from start seconds; the answer detaches. */
export async function attachHLS(video: HTMLVideoElement, url: string, native: boolean, start = 0): Promise<() => void> {
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
	// codec mid-play stalls the picture. Keep H.264 only and start near 720p.
	hls.on(Hls.Events.MANIFEST_PARSED, () => {
		const { drop, start: at } = planLevels(hls.levels.map((l) => ({ codec: l.videoCodec ?? l.codecSet ?? '', height: l.height })));
		for (const i of drop) hls.removeLevel(i);
		if (at >= 0) hls.startLevel = hls.nextLevel = at;
	});
	// It also carries auto-dubbed audio next to the original: keep the original.
	hls.on(Hls.Events.AUDIO_TRACKS_UPDATED, () => {
		const i = originalAudio(hls.audioTracks.map((t) => ({ name: t.name ?? '', isDefault: !!t.default })));
		if (i >= 0 && hls.audioTrack !== i) hls.audioTrack = i;
	});
	hls.on(Hls.Events.ERROR, (_e, d) => {
		if (d.fatal) video.dispatchEvent(new Event('error'));
	});
	if (typeof location !== 'undefined' && location.search.includes('trailerdebug')) (window as unknown as { __hls?: unknown[] }).__hls = [...((window as unknown as { __hls?: unknown[] }).__hls ?? []), hls];
	hls.loadSource(url);
	hls.attachMedia(video);
	return () => hls.destroy();
}

/** planLevels keeps H.264 levels only (when there are any): the indexes to
 * drop, highest first, and the start level among those left (the tallest at
 * or under 720p, else the shortest). */
export function planLevels(levels: { codec: string; height: number }[]): { drop: number[]; start: number } {
	const avc = (c: string) => c.toLowerCase().startsWith('avc1');
	const keep = levels.map((l, i) => ({ ...l, i })).filter((l) => avc(l.codec));
	if (!keep.length) return { drop: [], start: -1 };
	const drop = levels.map((l, i) => (avc(l.codec) ? -1 : i)).filter((i) => i >= 0).reverse();
	let start = -1;
	let best = -1;
	keep.forEach((l, k) => {
		if (l.height <= 720 && l.height > best) {
			best = l.height;
			start = k;
		}
	});
	return { drop, start: start >= 0 ? start : 0 };
}

/** originalAudio picks the track that is not an auto-dub: the default one, else the first such. */
export function originalAudio(tracks: { name: string; isDefault: boolean }[]): number {
	const real = (t: { name: string }) => !/dubbed|auto-?dub/i.test(t.name);
	const d = tracks.findIndex((t) => t.isDefault && real(t));
	return d >= 0 ? d : tracks.findIndex(real);
}
