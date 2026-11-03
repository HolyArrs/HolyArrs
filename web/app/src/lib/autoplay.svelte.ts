// The app-wide "Autoplay trailers" setting and the trailer sources a
// player tries in order: our own <video> on the direct stream, then the
// privacy-enhanced YouTube embed, then nothing (Layer 1 pictures stay).
import { api, ApiError } from './api.ts';
import { session } from './session.svelte.ts';
import { autoplayPref, browserConditions, finePointer, mayAutoplay, pickTrailer, soundOn, type Video } from './trailer.ts';
import { prefetchQueue } from './trailer-prefetch.ts';
import type { HeroVideo } from './music/hero-video.ts';

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

/** uiAutoplaySetting is the owner's "Autoplay trailers" setting (on until loaded). */
export function uiAutoplaySetting(): boolean {
	return state.autoplay;
}

/** autoplayHere says whether Layer 2 may start in this browser for this person. */
export function autoplayHere(): boolean {
	return state.loaded && mayAutoplay({ setting: state.autoplay, kid: session.kid, pref: autoplayPref(), ...browserConditions() });
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
		if (document.createElement('video').canPlayType('application/vnd.apple.mpegurl') === '') return false;
		// Chrome now answers "maybe" too, but its own HLS starts at the lowest
		// quality and stalls on YouTube's mixed streams; only Apple's (Safari,
		// iOS) is trusted. Elsewhere hls.js plays it.
		return /apple/i.test(navigator.vendor ?? '') || typeof MediaSource === 'undefined';
	} catch {
		return false;
	}
}

/** playable picks the address this browser plays (null: none). */
export function playable(s: Pick<Stream, 'type' | 'url' | 'mp4_url'>, hls = nativeHLS()): string | null {
	if (s.type === 'hls') return hls ? s.url : (s.mp4_url ?? null);
	return s.url || null;
}

// Warmed players: at hover start a hidden, muted <video> starts playing the
// address and holds on its first frame, so it is ready when the card opens;
// the card's player takes that element over (adoptWarm) and plays it. An address this
// browser refuses (Chrome may refuse YouTube's HLS manifest: error 4) is
// remembered, the next one is tried in the same element, and the card's
// player skips it.
interface Warm { v: HTMLVideoElement; url: string; t: ReturnType<typeof setTimeout>; dead?: boolean }
const warmed = new Map<string, Warm>();
const refused = new Set<string>();
function drop(v: HTMLVideoElement) {
	v.remove();
	v.pause();
	v.removeAttribute('src');
	v.load();
}
function forget(w: Warm) {
	// Emptying the element fires its error event: that is not a refusal.
	w.dead = true;
	clearTimeout(w.t);
	for (const [k, x] of warmed) if (x === w) warmed.delete(k);
}
function warm(urls: string[]) {
	const list = urls.filter((u) => !refused.has(u));
	if (typeof document === 'undefined' || !list.length || list.some((u) => warmed.has(u))) return;
	const v = document.createElement('video');
	v.muted = true;
	v.playsInline = true;
	v.loop = true;
	v.preload = 'auto';
	// In the page (Chrome plays native HLS only in an attached element), unseen.
	v.setAttribute('aria-hidden', 'true');
	v.style.cssText = 'position:fixed;left:0;top:0;width:160px;height:90px;opacity:0.01;pointer-events:none;z-index:-1';
	document.body.appendChild(v);
	const w: Warm = { v, url: list[0], t: setTimeout(() => (forget(w), drop(v)), 6_000) };
	let i = 0;
	const go = () => {
		w.url = list[i];
		v.src = w.url;
		void v.play().catch(() => {});
	};
	v.addEventListener('error', () => {
		if (w.dead || v.getAttribute('src') !== w.url) return;
		refused.add(w.url);
		warmed.delete(w.url);
		if (++i < list.length) go();
		else (forget(w), drop(v));
	});
	// Decoding has started once it plays: hold it on its first frame, so an
	// unseen player never keeps streaming 720p next to the hero and cards.
	v.addEventListener('playing', () => {
		if (!w.dead && v.isConnected && v.parentElement === document.body && v.getAttribute('aria-hidden') === 'true') v.pause();
	});
	for (const u of list) warmed.set(u, w);
	go();
	const all = new Set(warmed.values());
	if (all.size > 3) {
		const old = all.values().next().value as Warm;
		forget(old);
		drop(old.v);
	}
}

/** hasWarm says whether a warmed element is ready to be taken for url. */
export function hasWarm(url: string): boolean {
	const w = warmed.get(url);
	return !!w && w.url === url && !w.v.error && !w.dead;
}

/** adoptWarm hands over the warmed, already playing element for url (null: none). */
export function adoptWarm(url: string): HTMLVideoElement | null {
	const w = warmed.get(url);
	if (!w || !hasWarm(url)) return null;
	forget(w);
	w.dead = false;
	return w.v;
}

/**
 * prefetchTrailer starts the lookup at hover start (not after the dwell)
 * and, once the address is known, loads its first bytes (warmUp: hover
 * cards, whose player takes the warmed video over; a hero never does, so
 * warming one there only downloads the trailer twice next to the page).
 */
export function prefetchTrailer(kind: 'movie' | 'show', tmdb: string, warmUp = true) {
	if (!tmdb) return;
	void loadUISettings().then(() => {
		if (!autoplayHere()) return;
		void directStream(kind, tmdb).then((s) => {
			if (s && warmUp) warm(sourcesFor(s, undefined, nativeHLS(), soundOn()).flatMap((x) => (x.kind === 'direct' ? [x.url] : [])));
		});
	});
}

const streams = new Map<string, Promise<Stream | null>>();
/** Why a title has no direct stream, when the server said (by kind:tmdb). */
const misses = new Map<string, TrailerMiss>();
/** The answer for a title with no trailer (kept, unlike a failed ask). */
const NONE = Symbol('no trailer') as unknown as Stream;

const fresh = (s: Stream | null | undefined) => !!s?.url && Date.parse(s.expires_at) > Date.now() + 60_000;

// In-view prefetch: cards on or near the screen for IN_VIEW_MS are named to the server
// in one batch; the known addresses come back and the rest are looked up
// ahead of time on the server, so the hover finds them ready.
export const IN_VIEW_MS = 300;
const batch = prefetchQueue({
	send: async (ids) => {
		const b = await api.get<{ streams: Record<string, Stream>; videos?: Record<string, HeroVideo>; pending: string[] }>(`/trailer/batch?ids=${ids.map(encodeURIComponent).join(',')}&language=${lang()}`);
		for (const [id, s] of Object.entries(b?.streams ?? {})) {
			if (fresh(s) && !streams.has(id)) streams.set(id, Promise.resolve(s));
		}
		for (const [id, v] of Object.entries(b?.videos ?? {})) {
			if (freshVideo(v) && !artistVideos.has(id)) artistVideos.set(id, Promise.resolve(v));
		}
		return (b?.pending ?? []).filter((id) => !streams.has(id) && !artistVideos.has(id));
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
	return tmdb ? watchId(el, `${kind}:${tmdb}`) : () => {};
}

/** watchArtistVideo is watchTrailer for an artist's music video (artist:mbid). */
export function watchArtistVideo(el: Element, mbid: string): () => void {
	return mbid ? watchId(el, `artist:${mbid}`) : () => {};
}

function watchId(el: Element, id: string): () => void {
	if (typeof IntersectionObserver === 'undefined' || !finePointer()) return () => {};
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
			// A 404 means the title has no trailer: remembered for the session so
			// a second player does not ask again, with the server's reason and
			// key when it gave them. Other failures may be asked again.
			.catch((e) => {
				if (!(e instanceof ApiError) || e.status !== 404) return null;
				const { reason, key } = e.body ?? {};
				if (typeof reason === 'string' && reason) misses.set(k, { reason, key: typeof key === 'string' ? key : '' });
				return NONE;
			})
			.finally(() => clearTimeout(t));
		p.then((s) => {
			if (!s) streams.delete(k);
		});
		p = p.then((s) => (s === NONE ? null : s));
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
	return sourcesFor(s, embedKey(misses.get(`${kind}:${tmdb}`), videos, lang()), nativeHLS(), soundOn()).filter((x) => x.kind !== 'direct' || !refused.has(x.url));
}

/**
 * TrailerMiss is the server's 404 for a lookup that found no stream: the
 * reader's reason and a YouTube video its player may still play ("" when
 * every trailer tried will not play here).
 */
export interface TrailerMiss {
	reason: string;
	key: string;
}

/** Reasons about one video (age, removed, blocked), not YouTube's door. */
const VIDEO_REASONS = new Set(['age_restricted', 'unavailable', 'sign_in', 'unplayable', 'content_check', 'bad_id']);

/**
 * embedKey is the video the embed fallback plays: the server's key when it
 * gave one, none when it says no trailer plays here (the backdrop stays),
 * else our own pick from the title's videos.
 */
export function embedKey(miss: TrailerMiss | undefined, videos: Video[] | undefined, language: string): string | undefined {
	if (miss?.key) return miss.key;
	if (miss && VIDEO_REASONS.has(miss.reason)) return undefined;
	return pickTrailer(videos, language)?.key;
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
	const directAudio = !!s && (s.type === 'hls' && u !== s.url ? !!s.mp4_audio : !!s.audio);
	// Outside Safari the relay plays through hls.js with sound in the same
	// stream; when the direct one is picture-only, start on the relay so
	// turning sound on is a plain unmute (switching streams mid-play stalled).
	if (s && pass && !hls && (sound || !directAudio)) out.push({ kind: 'pass', url: pass, key: s.key });
	if (s && u) out.push({ kind: 'direct', url: u, key: s.key, audio: directAudio, pass });
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

// Artists' music videos go the trailers' way: named in view (the batch),
// looked up at hover start, and kept here for the artist page, whose hero
// then starts without waiting on the lookup.
const artistVideos = new Map<string, Promise<HeroVideo | null>>();

const freshVideo = (v: HeroVideo | null | undefined) => !!v?.proxy_url && (!v.expires_at || Date.parse(v.expires_at) > Date.now() + 60_000);

/** artistVideo asks for the artist's music video once (null: none). */
export function artistVideo(mbid: string, signal?: AbortSignal): Promise<HeroVideo | null> {
	const k = `artist:${mbid}`;
	let p = artistVideos.get(k);
	if (p) {
		const was = p;
		p = p.then((v) => {
			if (freshVideo(v)) return v;
			if (artistVideos.get(k) === was) artistVideos.delete(k);
			return v ? artistVideo(mbid, signal) : null;
		});
		return p;
	}
	const ctl = new AbortController();
	const t = setTimeout(() => ctl.abort(), 30_000);
	signal?.addEventListener('abort', () => ctl.abort(), { once: true });
	p = api
		.get<HeroVideo>(`/music/artists/${encodeURIComponent(mbid)}?part=video`, ctl.signal)
		.then((v) => (freshVideo(v) ? v : null))
		.catch(() => null)
		.finally(() => clearTimeout(t));
	const mine = p;
	p.then((v) => {
		if (!v && artistVideos.get(k) === mine) artistVideos.delete(k);
	});
	artistVideos.set(k, p);
	return p;
}

/** prefetchArtistVideo is prefetchTrailer for an artist: the lookup starts at hover start. */
export function prefetchArtistVideo(mbid: string) {
	if (!mbid) return;
	void loadUISettings().then(() => {
		if (autoplayHere()) void artistVideo(mbid);
	});
}
