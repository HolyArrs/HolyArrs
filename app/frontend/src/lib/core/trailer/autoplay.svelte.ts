// Autoplay setting, trailer lookups and warmed players, with the ordered sources a player tries.
import { api, ApiError } from '../api.ts';
import { session } from '../session.svelte.ts';
import { minimalLook } from '../look.svelte.ts';
import { autoplayPref, browserConditions, finePointer, mayAutoplay, pickTrailer, soundOn, type Video } from './trailer.ts';
import { prefetchQueue } from './trailer-prefetch.ts';
import type { HeroVideo } from './hero-video.ts';

export interface UISettings {
	autoplay_trailers: boolean;
}

// direct plays the address as is (pass is the same trailer relayed, for when sound goes on).
// pass is the relayed HLS stream. embed is the YouTube player, with the server's reason if no stream plays.
export type TrailerSource =
	| { kind: 'direct'; url: string; key?: string; audio: boolean; pass?: string }
	| { kind: 'pass'; url: string; key?: string }
	| { kind: 'embed'; key: string; why?: string; told?: boolean };

interface Stream {
	type?: 'hls' | 'mp4';
	url: string;
	mp4_url?: string;
	mp4_audio?: boolean;
	expires_at: string;
	key: string;
	audio?: boolean;
	proxy_url?: string;
}

interface Warm { v: HTMLVideoElement; url: string; t: ReturnType<typeof setTimeout>; dead?: boolean }

// The server's 404 for a title with no stream: why, and a video the embed may still play.
export interface TrailerMiss {
	reason: string;
	key: string;
}

const state = $state({ loaded: false, autoplay: true });
let loading: Promise<void> | null = null;

// Hidden players already started for a hover, and addresses this browser refused.
const warmed = new Map<string, Warm>();
const refused = new Set<string>();

const streams = new Map<string, Promise<Stream | null>>();
const misses = new Map<string, TrailerMiss>();
// A missing trailer is kept as an answer, unlike a failed ask, so nobody asks twice.
const NONE = Symbol('no trailer') as unknown as Stream;

// An address close to expiry is as good as gone.
const fresh = (s: Stream | null | undefined) => !!s?.url && Date.parse(s.expires_at) > Date.now() + 60_000;

// A card must stay on screen this long before its title is named to the server.
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

// Reasons about one video (age, removed, blocked), not about YouTube itself.
const VIDEO_REASONS = new Set(['age_restricted', 'unavailable', 'sign_in', 'unplayable', 'content_check', 'bad_id']);

// Music videos follow the trailer path: named in view, looked up on hover, kept for the artist page.
const artistVideos = new Map<string, Promise<HeroVideo | null>>();

const freshVideo = (v: HeroVideo | null | undefined) => !!v?.proxy_url && (!v.expires_at || Date.parse(v.expires_at) > Date.now() + 60_000);

export function loadUISettings(): Promise<void> {
	loading ??= api
		.get<UISettings>('/settings/ui')
		.then((s) => {
			state.autoplay = s.autoplay_trailers !== false;
		})
		.catch(() => {})
		.finally(() => {
			state.loaded = true;
		});
	return loading;
}

export function setAutoplaySetting(on: boolean) {
	state.autoplay = on;
	state.loaded = true;
}

export function autoplayHere(): boolean {
	return state.loaded && mayAutoplay({ setting: state.autoplay, kid: session.kid, pref: autoplayPref(), minimal: minimalLook(), ...browserConditions() });
}

// Chrome also says "maybe" to HLS, but its player starts at the lowest quality and stalls on YouTube's mixed streams.
// Elsewhere the MP4 plays, because YouTube's servers send no CORS headers for a script player.
export function nativeHLS(): boolean {
	if (typeof document === 'undefined') return false;
	try {
		if (document.createElement('video').canPlayType('application/vnd.apple.mpegurl') === '') return false;
		return /apple/i.test(navigator.vendor ?? '') || typeof MediaSource === 'undefined';
	} catch {
		return false;
	}
}

export function playable(s: Pick<Stream, 'type' | 'url' | 'mp4_url'>, hls = nativeHLS()): string | null {
	if (s.type === 'hls') return hls ? s.url : (s.mp4_url ?? null);
	return s.url || null;
}

export function hasWarm(url: string): boolean {
	const w = warmed.get(url);
	return !!w && w.url === url && !w.v.error && !w.dead;
}

// The card's player takes over the already playing element, so it starts with no wait.
export function adoptWarm(url: string): HTMLVideoElement | null {
	const w = warmed.get(url);
	if (!w || !hasWarm(url)) return null;
	forget(w);
	w.dead = false;
	return w.v;
}

// Starts the lookup at hover start, not after the dwell. Only hover cards warm the video:
// a hero never adopts it, so warming there would download the trailer twice.
export function prefetchTrailer(kind: 'movie' | 'show', tmdb: string, warmUp = true) {
	if (!tmdb) return;
	void loadUISettings().then(() => {
		if (!autoplayHere()) return;
		void directStream(kind, tmdb).then((s) => {
			if (s && warmUp) warm(sourcesFor(s, undefined, nativeHLS(), soundOn()).flatMap((x) => (x.kind === 'direct' ? [x.url] : [])));
		});
	});
}

// Only for desktop pointers where autoplay is allowed, so Kid accounts, reduced motion and saved data never prefetch.
export function watchTrailer(el: Element, kind: 'movie' | 'show', tmdb: string): () => void {
	return tmdb ? watchId(el, `${kind}:${tmdb}`) : () => {};
}

export function watchArtistVideo(el: Element, mbid: string): () => void {
	return mbid ? watchId(el, `artist:${mbid}`) : () => {};
}

export async function trailerSources(kind: 'movie' | 'show', tmdb: string, videos: Video[] | undefined, signal?: AbortSignal): Promise<TrailerSource[]> {
	const s = await directStream(kind, tmdb, signal);
	const miss = misses.get(`${kind}:${tmdb}`);
	return sourcesFor(s, embedKey(miss, videos, lang()), nativeHLS(), soundOn(), miss?.reason).filter((x) => x.kind !== 'direct' || !refused.has(x.url));
}

// A Trailer button must never do nothing: with no source left it opens the player, which says why.
export function clickSources(s: TrailerSource[], why: string, videos: Video[] | undefined, language: string): TrailerSource[] {
	if (s.length) return s;
	const v = pickTrailer(videos, language);
	if (!v) return [];
	// A reason about the video means it will not play here, so say so at once instead of loading YouTube's refusal.
	if (videoReason(why)) return [{ kind: 'embed', key: v.key, why, told: true }];
	return [why ? { kind: 'embed', key: v.key, why } : { kind: 'embed', key: v.key }];
}

export async function trailerClickSources(kind: 'movie' | 'show', tmdb: string, videos: Video[] | undefined): Promise<TrailerSource[]> {
	return clickSources(await trailerSources(kind, tmdb, videos), missReason(kind, tmdb), videos, lang());
}

export function missReason(kind: 'movie' | 'show', tmdb: string): string {
	return misses.get(`${kind}:${tmdb}`)?.reason ?? '';
}

export function videoReason(why: string | undefined): boolean {
	return !!why && VIDEO_REASONS.has(why);
}

// Server key first, then nothing when it says no trailer plays here (the backdrop stays), else our own pick.
export function embedKey(miss: TrailerMiss | undefined, videos: Video[] | undefined, language: string): string | undefined {
	if (miss?.key) return miss.key;
	if (miss && VIDEO_REASONS.has(miss.reason)) return undefined;
	return pickTrailer(videos, language)?.key;
}

// Direct comes first: muted and no server bandwidth. Relay is the fallback when direct is refused,
// since YouTube ties addresses to the IP that resolved them. The embed is last.
export function sourcesFor(s: Stream | null, videoKey: string | undefined, hls: boolean, sound: boolean, why?: string): TrailerSource[] {
	const out: TrailerSource[] = [];
	const u = s && playable(s, hls);
	const pass = s?.type === 'hls' ? s.proxy_url : undefined;
	const directAudio = !!s && (s.type === 'hls' && u !== s.url ? !!s.mp4_audio : !!s.audio);
	// Outside Safari the relay carries sound in the same stream. When direct is picture-only, start on the relay
	// so turning sound on is a plain unmute, because switching streams mid-play stalled.
	if (s && pass && !hls && (sound || !directAudio)) out.push({ kind: 'pass', url: pass, key: s.key });
	if (s && u) out.push({ kind: 'direct', url: u, key: s.key, audio: directAudio, pass });
	// Some browsers claim HLS and then refuse YouTube's manifest, so the MP4 is tried next.
	if (s?.type === 'hls' && u === s.url && s.mp4_url) out.push({ kind: 'direct', url: s.mp4_url, key: s.key, audio: !!s.mp4_audio, pass });
	if (s && pass && !out.some((o) => o.kind === 'pass')) out.push({ kind: 'pass', url: pass, key: s.key });
	const key = s?.key || videoKey;
	if (key) out.push(!s && why ? { kind: 'embed', key, why } : { kind: 'embed', key });
	return out;
}

// Unmute in place, switch to the relay (picture-only stream with a relay beside it), or nothing.
export function onUnmute(src: TrailerSource | undefined): 'unmute' | 'switch' | 'none' {
	if (!src) return 'none';
	if (src.kind !== 'direct' || src.audio) return 'unmute';
	return src.pass ? 'switch' : 'none';
}

export function hasSound(src: TrailerSource | undefined): boolean {
	return onUnmute(src) !== 'none';
}

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

export function prefetchArtistVideo(mbid: string) {
	if (!mbid) return;
	void loadUISettings().then(() => {
		if (autoplayHere()) void artistVideo(mbid);
	});
}

function drop(v: HTMLVideoElement) {
	v.remove();
	v.pause();
	v.removeAttribute('src');
	v.load();
}

function forget(w: Warm) {
	// Emptying the element fires its error event, which is not a refusal.
	w.dead = true;
	clearTimeout(w.t);
	for (const [k, x] of warmed) if (x === w) warmed.delete(k);
}

// A hidden muted video starts on the address and holds on its first frame, so it is ready when the card opens.
// An address the browser refuses is remembered and the next one is tried in the same element.
function warm(urls: string[]) {
	const list = urls.filter((u) => !refused.has(u));
	if (typeof document === 'undefined' || !list.length || list.some((u) => warmed.has(u))) return;
	const v = document.createElement('video');
	v.muted = true;
	v.playsInline = true;
	v.loop = true;
	v.preload = 'auto';
	// It must sit in the page, because Chrome plays native HLS only in an attached element.
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
	// Once it plays, decoding has started: pause so an unseen player never keeps streaming 720p.
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

// Kept outside watchId on purpose: made inside, it shared a scope with the clean-up,
// which holds the first watched card, so the observer kept that card's whole page alive.
function onSeen(entries: IntersectionObserverEntry[]) {
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
}

function watchId(el: Element, id: string): () => void {
	if (typeof IntersectionObserver === 'undefined' || !finePointer()) return () => {};
	// Rows just below the screen count too, because a cold lookup takes a few seconds.
	seen ??= new IntersectionObserver(onSeen, { rootMargin: '300px 0px 600px 0px', threshold: 0 });
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
			// A 404 means no trailer, remembered with the server's reason and key so a second player does not ask again.
			// A title not looked up yet comes with its key alone, for the embed. Other failures may be asked again.
			.catch((e) => {
				if (!(e instanceof ApiError) || e.status !== 404) return null;
				const { reason, key } = e.body ?? {};
				if ((typeof reason === 'string' && reason) || (typeof key === 'string' && key)) misses.set(k, { reason: typeof reason === 'string' ? reason : '', key: typeof key === 'string' ? key : '' });
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
