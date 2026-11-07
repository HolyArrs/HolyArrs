// Pass-through playback: HolyArrs' relayed HLS stream on a <video>. Native
// HLS (Safari) plays it directly; elsewhere hls.js feeds it. hls.js ships
// as its own file (scripts/vendor-hls.mjs copies the pinned version) and is
// loaded only when first needed. A fatal hls.js error becomes the video's
// error event, so players treat both the same way.
import type HlsType from 'hls.js';
import { to } from '../paths.ts';

/** The pinned hls.js (package.json); a test checks they agree. */
export const HLS_VERSION = '1.7.3';
export const hlsScript = `/vendor/hls-${HLS_VERSION}.min.js`;

/**
 * BUFFER caps how far ahead hls.js reads. maxBufferLength alone did not hold:
 * hls.js grows the goal toward maxMaxBufferLength (600 s by default) whenever
 * maxBufferSize (60 MB) is not reached, keeps an unlimited back buffer, and
 * fetches the next fragment early. Every limit is set so a hero or row video
 * reads about 20 s ahead and drops what it has played.
 */
export const BUFFER = {
	maxBufferLength: 20,
	maxMaxBufferLength: 30,
	maxBufferSize: 15 * 1000 * 1000,
	backBufferLength: 10,
	startFragPrefetch: false
} as const;

let loading: Promise<typeof HlsType> | null = null;
function loadHls(): Promise<typeof HlsType> {
	loading ??= (import(/* @vite-ignore */ to(hlsScript)) as Promise<{ default: typeof HlsType }>).then((m) => m.default);
	loading.catch(() => (loading = null));
	return loading;
}

/** attachHLS plays url on video from start seconds; the answer detaches.
 * target is the tallest picture worth fetching (playerTarget; hover cards keep 720).
 * floor keeps the picture at least that tall whatever the player's size (a
 * retry at the largest picture, when a smaller one would not decode). */
export async function attachHLS(video: HTMLVideoElement, url: string, native: boolean, start = 0, target = 720, floor = 0): Promise<() => void> {
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
	const hls = new Hls({ startPosition: start, ...BUFFER, capLevelToPlayerSize: false, enableWorker: true, abrEwmaDefaultEstimate: 8_000_000 });
	// YouTube's stream mixes H.264, VP9 and AV1 at the same sizes; switching
	// codec mid-play stalls the picture, so only H.264 stays. The cap is what
	// the player shows right now (its box, or the screen in full screen) and
	// follows it when it grows or shrinks.
	let avc: { height: number }[] = [];
	const recap = () => {
		if (!avc.length) return;
		const cap = capLevel(avc.map((l) => l.height), Math.max(shownHeight(video), floor));
		if (hls.autoLevelCapping !== cap) hls.autoLevelCapping = cap;
	};
	hls.on(Hls.Events.MANIFEST_PARSED, () => {
		const { drop } = planLevels(hls.levels.map((l) => ({ codec: l.videoCodec ?? l.codecSet ?? '', height: l.height })), target);
		for (const i of drop) hls.removeLevel(i);
		avc = hls.levels.map((l) => ({ height: l.height }));
		if (!avc.length) return;
		// Start straight at the level for what is shown (a light start stayed
		// low too long); full screen and resizes still move the cap.
		recap();
		hls.startLevel = hls.nextLevel = hls.autoLevelCapping;
	});
	const ro = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(recap);
	ro?.observe(video);
	document.addEventListener('fullscreenchange', recap);
	window.addEventListener('resize', recap);
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
	return () => {
		ro?.disconnect();
		document.removeEventListener('fullscreenchange', recap);
		window.removeEventListener('resize', recap);
		hls.destroy();
	};
}

/**
 * holdStart keeps a video meant to start at start (seconds) from playing its
 * opening instead: whenever it plays more than slack seconds before the start,
 * it jumps there. Safari's own HLS can drop the seek made at loadedmetadata,
 * and a stream that starts over can land at 0. A seek made in the video's own
 * player (iPhone full screen, native controls) is the person's: from then on
 * it is left alone. Three jumps that do not hold (a start the stream cannot
 * reach) and it gives up until the video gets past the start again. The
 * answer stops it.
 */
export function holdStart(video: HTMLVideoElement, start: number, slack = 2): () => void {
	if (!(start > slack)) return () => {};
	let theirs = false;
	let jumping = false;
	let tries = 0;
	const ownPlayer = () =>
		video.controls ||
		!!(video as HTMLVideoElement & { webkitDisplayingFullscreen?: boolean }).webkitDisplayingFullscreen ||
		(typeof document !== 'undefined' && document.fullscreenElement === video);
	const seeking = () => {
		if (jumping) jumping = false;
		else if (ownPlayer()) theirs = true;
	};
	const check = () => {
		if (theirs || video.paused || video.seeking) return;
		if (video.currentTime >= start - slack) tries = 0;
		else if (tries < 3) {
			tries++;
			jumping = true;
			video.currentTime = start;
		}
	};
	video.addEventListener('seeking', seeking);
	video.addEventListener('playing', check);
	video.addEventListener('timeupdate', check);
	return () => {
		video.removeEventListener('seeking', seeking);
		video.removeEventListener('playing', check);
		video.removeEventListener('timeupdate', check);
	};
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

/** shownHeight is the picture height the video shows now, in device pixels:
 * the screen in full screen, else its box (filled at 16:9). */
export function shownHeight(video: HTMLVideoElement): number {
	const ratio = globalThis.devicePixelRatio || 1;
	const full = typeof document !== 'undefined' && document.fullscreenElement && (document.fullscreenElement === video || document.fullscreenElement.contains(video));
	const w = full ? (globalThis.screen?.width ?? video.clientWidth) : video.clientWidth;
	const h = full ? (globalThis.screen?.height ?? video.clientHeight) : video.clientHeight;
	return Math.ceil(Math.max(h, (w * 9) / 16) * ratio);
}

/** lightLevel is the quick-start level: the first at least 360 tall, else the shortest. */
export function lightLevel(heights: number[]): number {
	const order = heights.map((h, i) => ({ h, i })).sort((a, b) => a.h - b.h);
	return (order.find((l) => l.h >= 360) ?? order[0])?.i ?? 0;
}

/** capLevel is the tallest level worth climbing to: the shortest that covers
 * what is shown, else the tallest there is. */
export function capLevel(heights: number[], shown: number): number {
	const order = heights.map((h, i) => ({ h, i })).sort((a, b) => a.h - b.h);
	return (order.find((l) => l.h >= shown) ?? order[order.length - 1])?.i ?? -1;
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
