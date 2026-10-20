// The app-wide "Autoplay trailers" setting and the trailer sources a
// player tries in order: our own <video> on the direct stream, then the
// privacy-enhanced YouTube embed, then nothing (Layer 1 pictures stay).
import { api } from './api.ts';
import { session } from './session.svelte.ts';
import { browserConditions, finePointer, mayAutoplay, pickTrailer, soundOn, type Video } from './trailer.ts';
import { prefetchQueue } from './trailer-prefetch.ts';

export interface UISettings {
	autoplay_trailers: boolean;
}

const state = $state({ loaded: false, autoplay: true });
let loading: Promise<void> | null = null;

/** loadUISettings reads GET /settings/ui once per page load. */
export function loadUISettings(): Promise<void> {
	loading ??= api
		.get<UISettings>('/settings/ui')
		.then((s) => {
			state.autoplay = s.autoplay_trailers !== false;
		})
		.catch(() => {
			/* the built-in default (on) stays */
		})
		.finally(() => {
			state.loaded = true;
		});
	return loading;
}

/** After a save on Settings. */
export function setAutoplaySetting(on: boolean) {
	state.autoplay = on;
	state.loaded = true;
}

export const uiSettings = {
	get autoplay() {
		return state.autoplay;
	},
	get loaded() {
		return state.loaded;
	}
};

/** autoplayHere says whether Layer 2 may start in this browser for this person. */
export function autoplayHere(): boolean {
	return state.loaded && mayAutoplay({ setting: state.autoplay, kid: session.kid, ...browserConditions() });
}

/**
 * direct: straight from YouTube (pass: the same trailer through HolyArrs,
 * for switching to when sound is turned on); pass: the HLS stream relayed
 * by HolyArrs (pass-through); embed: the YouTube embed.
 */
export type TrailerSource =
	| { kind: 'direct'; url: string; key?: string; audio: boolean; pass?: string }
	| { kind: 'pass'; url: string; key?: string }
	| { kind: 'embed'; key: string };

interface Stream {
	/** hls: url is YouTube's adaptive manifest; mp4: url is the muxed 360p MP4. */
	type?: 'hls' | 'mp4';
	url: string;
	/** The MP4 beside an HLS answer, and whether it has sound. */
	mp4_url?: string;
	mp4_audio?: boolean;
	expires_at: string;
	key: string;
	audio?: boolean;
	/** The same HLS stream relayed by HolyArrs (pass-through). */
	proxy_url?: string;
}

/**
 * Native HLS (Safari, iOS). Elsewhere the MP4 plays: YouTube's video
 * servers send no CORS headers, so a script player cannot read the
 * manifest or its segments; with sound on, HolyArrs relays the HLS stream.
 */
export function nativeHLS(): boolean {
	if (typeof document === 'undefined') return false;
	try {
		return document.createElement('video').canPlayType('application/vnd.apple.mpegurl') !== '';
	} catch {
		return false;
	}
}

/** playable picks the address this browser plays (null: none). */
export function playable(s: Pick<Stream, 'type' | 'url' | 'mp4_url'>, hls = nativeHLS()): string | null {
	if (s.type === 'hls') return hls ? s.url : (s.mp4_url ?? null);
	return s.url || null;
}

// Warmed players: the first bytes (MP4 header, or the HLS manifest) load
// while the dwell runs, so the real player starts from the browser cache.
const warmed = new Map<string, HTMLVideoElement>();
function warm(url: string) {
	if (typeof document === 'undefined' || warmed.has(url)) return;
	const v = document.createElement('video');
	v.muted = true;
	// auto: the HLS manifest and first segments (or the MP4's first
	// bytes) are in the cache before the card opens.
	v.preload = 'auto';
	v.src = url;
	warmed.set(url, v);
	setTimeout(() => {
		v.removeAttribute('src');
		v.load();
		warmed.delete(url);
	}, 30_000);
	while (warmed.size > 4) {
		const [k, old] = warmed.entries().next().value as [string, HTMLVideoElement];
		old.removeAttribute('src');
		old.load();
		warmed.delete(k);
	}
}

/**
 * prefetchTrailer starts the lookup at hover start (not after the dwell)
 * and, once the address is known, loads its first bytes.
 */
export function prefetchTrailer(kind: 'movie' | 'show', tmdb: string) {
	if (!tmdb) return;
	void loadUISettings().then(() => {
		if (!autoplayHere()) return;
		void directStream(kind, tmdb).then((s) => {
			const u = s && playable(s);
			if (u) warm(u);
		});
	});
}

const streams = new Map<string, Promise<Stream | null>>();

const fresh = (s: Stream | null | undefined) => !!s?.url && Date.parse(s.expires_at) > Date.now() + 60_000;

// In-view prefetch: cards on or near the screen for IN_VIEW_MS are named to the server
// in one batch; the known addresses come back and the rest are looked up
// ahead of time on the server, so the hover finds them ready.
export const IN_VIEW_MS = 300;
const batch = prefetchQueue({
	send: async (ids) => {
		const b = await api.get<{ streams: Record<string, Stream>; pending: string[] }>(`/trailer/batch?ids=${ids.map(encodeURIComponent).join(',')}&language=${lang()}`);
		for (const [id, s] of Object.entries(b?.streams ?? {})) {
			if (fresh(s) && !streams.has(id)) streams.set(id, Promise.resolve(s));
		}
		return (b?.pending ?? []).filter((id) => !streams.has(id));
	}
});
let seen: IntersectionObserver | null = null;
const viewTimers = new WeakMap<Element, ReturnType<typeof setTimeout>>();
const viewIds = new WeakMap<Element, string>();

/**
 * watchTrailer names this card's title for the in-view prefetch once it has
 * been on screen for IN_VIEW_MS (desktop pointer, autoplay allowed here:
 * never for Kid accounts, reduced motion or saved data). The answer stops watching.
 */
export function watchTrailer(el: Element, kind: 'movie' | 'show', tmdb: string): () => void {
	if (!tmdb || typeof IntersectionObserver === 'undefined' || !finePointer()) return () => {};
	const id = `${kind}:${tmdb}`;
	seen ??= new IntersectionObserver(
		(entries) => {
			for (const e of entries) {
				const t = viewTimers.get(e.target);
				if (e.isIntersecting && t === undefined) {
					const target = e.target;
					viewTimers.set(
						target,
						setTimeout(() => {
							const tid = viewIds.get(target);
							if (!tid || streams.has(tid)) return;
							void loadUISettings().then(() => autoplayHere() && batch.add(tid));
						}, IN_VIEW_MS)
					);
				} else if (!e.isIntersecting && t !== undefined) {
					clearTimeout(t);
					viewTimers.delete(e.target);
				}
			}
		},
		// Rows just below the screen count too: a cold lookup takes 1-3 s.
		{ rootMargin: '300px 0px 600px 0px', threshold: 0 }
	);
	viewIds.set(el, id);
	seen.observe(el);
	return () => {
		const t = viewTimers.get(el);
		if (t !== undefined) clearTimeout(t);
		viewTimers.delete(el);
		viewIds.delete(el);
		seen?.unobserve(el);
	};
}

function lang(): string {
	return (typeof navigator !== 'undefined' ? navigator.language : 'en').split('-')[0] || 'en';
}

/** directStream asks the server for the direct trailer address (null: none). */
function directStream(kind: 'movie' | 'show', tmdb: string, signal?: AbortSignal): Promise<Stream | null> {
	const k = `${kind}:${tmdb}`;
	let p = streams.get(k);
	if (!p) {
		const ctl = new AbortController();
		const t = setTimeout(() => ctl.abort(), 9000);
		signal?.addEventListener('abort', () => ctl.abort(), { once: true });
		p = api
			.get<Stream>(`/trailer/${kind}/${encodeURIComponent(tmdb)}?language=${lang()}`, ctl.signal)
			.then((s) => (fresh(s) ? s : null))
			.catch(() => null)
			.finally(() => clearTimeout(t));
		p.then((s) => {
			if (!s) streams.delete(k);
		});
		streams.set(k, p);
	}
	return p;
}

/**
 * trailerSources lists what a player tries, in order. videos are the
 * title's TMDB videos (from the details answer) for the embed fallback.
 */
export async function trailerSources(kind: 'movie' | 'show', tmdb: string, videos: Video[] | undefined, signal?: AbortSignal): Promise<TrailerSource[]> {
	const s = await directStream(kind, tmdb, signal);
	return sourcesFor(s, pickTrailer(videos, lang())?.key, nativeHLS(), soundOn());
}

/**
 * sourcesFor orders the sources. Direct from YouTube first (muted, no
 * server bandwidth); once sound was turned on this session, a browser
 * without native HLS starts on pass-through instead. Pass-through is also
 * the fallback when direct playback is refused (YouTube ties addresses to
 * the IP that resolved them), then the embed.
 */
export function sourcesFor(s: Stream | null, videoKey: string | undefined, hls: boolean, sound: boolean): TrailerSource[] {
	const out: TrailerSource[] = [];
	const u = s && playable(s, hls);
	const pass = s?.type === 'hls' ? s.proxy_url : undefined;
	if (s && pass && sound && !hls) out.push({ kind: 'pass', url: pass, key: s.key });
	if (s && u) out.push({ kind: 'direct', url: u, key: s.key, audio: s.type === 'hls' && u !== s.url ? !!s.mp4_audio : !!s.audio, pass });
	// Some browsers say they may play HLS and then refuse YouTube's manifest
	// (Chrome: error 4); the MP4 is tried next.
	if (s?.type === 'hls' && u === s.url && s.mp4_url) out.push({ kind: 'direct', url: s.mp4_url, key: s.key, audio: !!s.mp4_audio, pass });
	if (s && pass && !out.some((o) => o.kind === 'pass')) out.push({ kind: 'pass', url: pass, key: s.key });
	const key = s?.key || videoKey;
	if (key) out.push({ kind: 'embed', key });
	return out;
}

/**
 * onUnmute says what turning the sound on does: unmute in place, switch to
 * pass-through (a picture-only stream with a relay beside it), or nothing.
 */
export function onUnmute(src: TrailerSource | undefined): 'unmute' | 'switch' | 'none' {
	if (!src) return 'none';
	if (src.kind !== 'direct' || src.audio) return 'unmute';
	return src.pass ? 'switch' : 'none';
}

/** hasSound says whether the sound button shows for this source. */
export function hasSound(src: TrailerSource | undefined): boolean {
	return onUnmute(src) !== 'none';
}
