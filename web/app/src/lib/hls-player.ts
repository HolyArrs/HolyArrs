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
	const hls = new Hls({ startPosition: start, maxBufferLength: 20, capLevelToPlayerSize: true, enableWorker: true });
	hls.on(Hls.Events.ERROR, (_e, d) => {
		if (d.fatal) video.dispatchEvent(new Event('error'));
	});
	hls.loadSource(url);
	hls.attachMedia(video);
	return () => hls.destroy();
}
