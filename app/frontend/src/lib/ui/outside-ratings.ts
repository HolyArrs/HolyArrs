import { to } from '../core/paths.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';

// Cleans outside scores and artwork from the extras endpoint and caches them.

export type OutsideScore = { source: string; value: number; max: number; votes: number };
export type Extras = { logo?: string; thumb?: string; backgrounds: string[]; scores: OutsideScore[] };

// Display order of sources; anything else is dropped.
const ORDER = ['imdb', 'rt_critics', 'rt_audience', 'metacritic', 'letterboxd', 'trakt'];
const NAMES: Record<string, string> = {
	imdb: 'IMDb',
	get rt_critics() {
		return t('ui.outsideRatings.rtCritics');
	},
	get rt_audience() {
		return t('ui.outsideRatings.rtAudience');
	},
	metacritic: 'Metacritic',
	letterboxd: 'Letterboxd',
	trakt: 'Trakt'
};
const EMPTY: Extras = { backgrounds: [], scores: [] };
const cache = new Map<string, Promise<Extras>>();

// Keeps only sane scores and our own artwork paths; never throws.
export function cleanExtras(raw: unknown): Extras {
	if (!raw || typeof raw !== 'object') return EMPTY;
	const r = raw as Record<string, unknown>;
	const scores = (Array.isArray(r.scores) ? r.scores : [])
		.filter((s): s is OutsideScore => !!s && ORDER.includes(s.source) && Number.isFinite(s.value) && Number.isFinite(s.max) && s.value > 0 && s.max > 0 && s.value <= s.max)
		.sort((a, b) => ORDER.indexOf(a.source) - ORDER.indexOf(b.source));
	const backgrounds = (Array.isArray(r.backgrounds) ? r.backgrounds : []).map(picture).filter((u): u is string => !!u);
	return { logo: picture(r.logo), thumb: picture(r.thumb), backgrounds, scores };
}

// Percent sources show a percent, Metacritic a bare number, the rest one decimal.
export function scoreText(s: OutsideScore): string {
	if (s.max === 100) return s.source === 'metacritic' ? String(Math.round(s.value)) : `${Math.round(s.value)}%`;
	return (Math.round(s.value * 10) / 10).toFixed(1);
}

export const scoreName = (source: string) => NAMES[source] ?? source;

// One request per title for the session; failures resolve to empty.
export function loadExtras(get: <T>(path: string) => Promise<T>, kind: 'movie' | 'show' | 'artist', id: string | null | undefined): Promise<Extras> {
	if (!id) return Promise.resolve(EMPTY);
	const key = `${kind}:${id}`;
	let p = cache.get(key);
	if (!p) {
		p = get<unknown>(`/extras/${kind}/${encodeURIComponent(id)}`).then(cleanExtras, () => EMPTY);
		cache.set(key, p);
	}
	return p;
}

// Only our own artwork URLs are trusted.
const ownArt = /^\/api\/v1\/artwork\/img\/[a-z-]+\/[a-z0-9-]+\/\d+\.(?:png|webp|jpg)(?:\?v=[A-Za-z0-9_-]+)?$/;
const picture = (v: unknown) => (typeof v === 'string' && ownArt.test(v) ? to(v) : undefined);
