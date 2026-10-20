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
	url: string;
	expires_at: string;
	key: string;
	audio?: boolean;
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
	if (s) out.push({ kind: 'direct', url: s.url, key: s.key, audio: !!s.audio });
	const key = s?.key || pickTrailer(videos, lang())?.key;
	if (key) out.push({ kind: 'embed', key });
	return out;
}
