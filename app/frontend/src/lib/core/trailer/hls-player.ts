// Plays the relayed HLS stream on a video, plus the level and start helpers around it.
import type HlsType from 'hls.js';
import { to } from '../paths.ts';

export const HLS_VERSION = '1.7.3';
export const hlsScript = `/vendor/hls-${HLS_VERSION}.min.js`;

// maxBufferLength alone does not hold: hls.js grows the goal toward its larger
// default and keeps an unlimited back buffer. All limits are set so a video
// reads about 20 s ahead and drops what it played.
export const BUFFER = {
	maxBufferLength: 20,
	maxMaxBufferLength: 30,
	maxBufferSize: 15 * 1000 * 1000,
	backBufferLength: 10,
	startFragPrefetch: false
} as const;

// target is the tallest picture worth fetching; floor is the least height to keep
// whatever the player's size (a retry when a smaller one would not decode).
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
	// The stream mixes codecs at the same sizes and switching codec mid-play stalls,
	// so only H.264 stays. The cap follows what the player shows now.
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
		// Starting low stayed low too long, so start at the shown level.
		recap();
		hls.startLevel = hls.nextLevel = hls.autoLevelCapping;
	});
	const ro = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(recap);
	ro?.observe(video);
	document.addEventListener('fullscreenchange', recap);
	window.addEventListener('resize', recap);
	// The stream also carries auto-dubbed audio; keep the original.
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

// Safari's HLS can drop the first seek, and a restarted stream can land at 0,
// so jump back whenever play is well before start. A seek in the video's own
// player is the viewer's and is left alone. Three jumps that do not hold means
// the start is unreachable, so it gives up.
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

// Cover fills the box, contain fits inside it. Capped at 1080 to never fetch 4K.
export function playerTarget(w: number, h: number, fit: 'cover' | 'contain', ratio = globalThis.devicePixelRatio || 1): number {
	const tall = fit === 'cover' ? Math.max(h, (w * 9) / 16) : Math.min(h, (w * 9) / 16);
	return Math.min(1080, Math.max(360, Math.ceil(tall * ratio)));
}

// Sized by the screen, not the window, or a small window would lock a low level that full screen keeps.
export function screenTarget(fit: 'cover' | 'contain', ratio = globalThis.devicePixelRatio || 1): number {
	const s = typeof screen === 'undefined' ? null : screen;
	const w = Math.max(s?.width ?? 0, globalThis.innerWidth || 0);
	const h = Math.max(s?.height ?? 0, globalThis.innerHeight || 0);
	return playerTarget(w || 1280, h || 720, fit, ratio);
}

// Device pixels: the screen in full screen, else the box filled at 16:9.
export function shownHeight(video: HTMLVideoElement): number {
	const ratio = globalThis.devicePixelRatio || 1;
	const full = typeof document !== 'undefined' && document.fullscreenElement && (document.fullscreenElement === video || document.fullscreenElement.contains(video));
	const w = full ? (globalThis.screen?.width ?? video.clientWidth) : video.clientWidth;
	const h = full ? (globalThis.screen?.height ?? video.clientHeight) : video.clientHeight;
	return Math.ceil(Math.max(h, (w * 9) / 16) * ratio);
}

// The shortest level that covers what is shown, else the tallest there is.
export function capLevel(heights: number[], shown: number): number {
	const order = heights.map((h, i) => ({ h, i })).sort((a, b) => a.h - b.h);
	return (order.find((l) => l.h >= shown) ?? order[order.length - 1])?.i ?? -1;
}

// Drops non-H.264 levels (highest index first) when any H.264 exists, and picks the start level.
export function planLevels(levels: { codec: string; height: number }[], target = 720): { drop: number[]; start: number } {
	target = Math.min(target, 1080);
	const avc = (c: string) => c.toLowerCase().startsWith('avc1');
	const keep = levels.map((l, i) => ({ ...l, i })).filter((l) => avc(l.codec));
	if (!keep.length) return { drop: [], start: -1 };
	const drop = levels.map((l, i) => (avc(l.codec) ? -1 : i)).filter((i) => i >= 0).reverse();
	// Prefer the shortest level that covers the target, else the tallest under it.
	const fits = keep.map((l, k) => ({ h: l.height, k })).filter((l) => l.h <= 1080);
	const cover = fits.filter((l) => l.h >= target).sort((a, b) => a.h - b.h)[0];
	const under = fits.sort((a, b) => b.h - a.h)[0];
	return { drop, start: (cover ?? under)?.k ?? 0 };
}

// Skips auto-dubbed tracks; prefers the default of the rest.
export function originalAudio(tracks: { name: string; isDefault: boolean }[]): number {
	const real = (t: { name: string }) => !/dubbed|auto-?dub/i.test(t.name);
	const d = tracks.findIndex((t) => t.isDefault && real(t));
	return d >= 0 ? d : tracks.findIndex(real);
}

// Loaded once, on first use; a failed load is retried next time.
let loading: Promise<typeof HlsType> | null = null;
function loadHls(): Promise<typeof HlsType> {
	loading ??= (import(/* @vite-ignore */ to(hlsScript)) as Promise<{ default: typeof HlsType }>).then((m) => m.default);
	loading.catch(() => (loading = null));
	return loading;
}
