// The app-wide "Autoplay trailers" setting and the trailer sources a
// player tries in order: our own <video> on the direct stream, then the
// privacy-enhanced YouTube embed, then nothing (Layer 1 pictures stay).
import { api } from './api.ts';
import { session } from './session.svelte.ts';
import { browserConditions, mayAutoplay, pickTrailer, type Video } from './trailer.ts';

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

export type TrailerSource = { kind: 'direct'; url: string; key?: string; audio: boolean } | { kind: 'embed'; key: string };

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
}

/**
 * Native HLS (Safari, iOS). Elsewhere the MP4 plays: YouTube's video
 * servers send no CORS headers, so a script player cannot read the
 * manifest or its segments.
 */
function nativeHLS(): boolean {
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
	v.preload = 'metadata';
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
			.then((s) => (s?.url && Date.parse(s.expires_at) > Date.now() + 60_000 ? s : null))
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
	const out: TrailerSource[] = [];
	const s = await directStream(kind, tmdb, signal);
	const u = s && playable(s);
	if (s && u) out.push({ kind: 'direct', url: u, key: s.key, audio: s.type === 'hls' && u !== s.url ? !!s.mp4_audio : !!s.audio });
	// Some browsers say they may play HLS and then refuse YouTube's manifest
	// (Chrome: error 4); the MP4 is tried next.
	if (s?.type === 'hls' && u === s.url && s.mp4_url) out.push({ kind: 'direct', url: s.mp4_url, key: s.key, audio: !!s.mp4_audio });
	const key = s?.key || pickTrailer(videos, lang())?.key;
	if (key) out.push({ kind: 'embed', key });
	return out;
}
