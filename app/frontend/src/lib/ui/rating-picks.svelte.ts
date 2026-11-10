// Which outside rating sources the user picked to show, kept in local storage.

import type { OutsideScore } from './outside-ratings.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';

export type RatingSource = 'imdb' | 'rt_critics' | 'rt_audience' | 'metacritic' | 'letterboxd' | 'trakt' | 'tmdb';

export const RATING_SOURCES: { id: RatingSource; name: string; about: string }[] = [
	{ id: 'imdb', name: 'IMDb', get about() { return t('ui.ratingPicks.imdb'); } },
	{ id: 'rt_critics', get name() { return t('ui.outsideRatings.rtCritics'); }, get about() { return t('ui.ratingPicks.rtCritics'); } },
	{ id: 'rt_audience', get name() { return t('ui.outsideRatings.rtAudience'); }, get about() { return t('ui.ratingPicks.rtAudience'); } },
	{ id: 'metacritic', name: 'Metacritic', get about() { return t('ui.ratingPicks.metacritic'); } },
	{ id: 'letterboxd', name: 'Letterboxd', get about() { return t('ui.ratingPicks.letterboxd'); } },
	{ id: 'trakt', name: 'Trakt', get about() { return t('ui.ratingPicks.trakt'); } },
	{ id: 'tmdb', name: 'TMDB', get about() { return t('ui.ratingPicks.tmdb'); } }
];

export const DEFAULT_RATINGS: RatingSource[] = ['imdb', 'rt_critics', 'rt_audience'];
const ORDER = RATING_SOURCES.map((s) => s.id);
const KEY = 'holyarrs.ratings';
export const MAX_RATINGS = 3;

// Storage can throw (private mode), so every access is guarded.
let saved = false;
try {
	saved = typeof window !== 'undefined' && localStorage.getItem(KEY) !== null;
} catch {
}
export const ratingPicks = $state<{ list: RatingSource[]; custom: boolean }>({ list: typeof window === 'undefined' ? DEFAULT_RATINGS : stored(), custom: saved });

// Stored in source order, and capped so cards stay readable.
export function setRatingPick(id: RatingSource, on: boolean) {
	if (on && !ratingPicks.list.includes(id) && ratingPicks.list.length >= MAX_RATINGS) return;
	const next = ORDER.filter((s) => (s === id ? on : ratingPicks.list.includes(s)));
	ratingPicks.list = next;
	ratingPicks.custom = true;
	try {
		localStorage.setItem(KEY, JSON.stringify(next));
	} catch {
	}
}

export function resetRatingPicks() {
	ratingPicks.list = DEFAULT_RATINGS;
	ratingPicks.custom = false;
	try {
		localStorage.removeItem(KEY);
	} catch {
	}
}

// With fill, empty slots take other sources; if nothing matches, show the first one available.
export function shownRatings(scores: OutsideScore[], tmdb: number | null | undefined, picks: RatingSource[], fill = false): { scores: OutsideScore[]; tmdb: number | undefined } {
	if (!fill && picks.length === 0) return { scores: [], tmdb: undefined };
	const out = scores.filter((s) => picks.includes(s.source as RatingSource));
	let t = tmdb && picks.includes('tmdb') ? tmdb : undefined;
	if (fill) {
		for (const s of scores.filter((x) => !out.includes(x)).sort((a, b) => ORDER.indexOf(a.source as RatingSource) - ORDER.indexOf(b.source as RatingSource))) {
			if (out.length >= MAX_RATINGS) break;
			out.push(s);
		}
		if (out.length < MAX_RATINGS && tmdb && !t) t = tmdb;
	}
	if (out.length || t) return { scores: out, tmdb: t };
	for (const id of ORDER) {
		if (id === 'tmdb' && tmdb) return { scores: [], tmdb };
		const s = scores.find((x) => x.source === id);
		if (s) return { scores: [s], tmdb: undefined };
	}
	return { scores: [], tmdb: undefined };
}

// Falls back to the defaults when nothing valid is stored.
function stored(): RatingSource[] {
	try {
		const raw = localStorage.getItem(KEY);
		if (!raw) return DEFAULT_RATINGS;
		const list = (JSON.parse(raw) as unknown[]).filter((x): x is RatingSource => ORDER.includes(x as RatingSource));
		return list;
	} catch {
		return DEFAULT_RATINGS;
	}
}
