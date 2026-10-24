// Which ratings a title page shows, picked per browser in Settings › Appearance.
// Three by default: IMDb (viewers), Rotten Tomatoes critics and audience. A title
// with none of the picked ones still shows one score, the first it has in ORDER.
import type { OutsideScore } from './outside-ratings.ts';

export type RatingSource = 'imdb' | 'rt_critics' | 'rt_audience' | 'metacritic' | 'letterboxd' | 'trakt' | 'tmdb';

export const RATING_SOURCES: { id: RatingSource; name: string; about: string }[] = [
	{ id: 'imdb', name: 'IMDb', about: 'Viewers’ average, out of 10' },
	{ id: 'rt_critics', name: 'Rotten Tomatoes critics', about: 'Share of critics who liked it' },
	{ id: 'rt_audience', name: 'Rotten Tomatoes audience', about: 'Share of viewers who liked it' },
	{ id: 'metacritic', name: 'Metacritic', about: 'Critics’ weighted score, out of 100' },
	{ id: 'letterboxd', name: 'Letterboxd', about: 'Film fans’ average, out of 5 (movies)' },
	{ id: 'trakt', name: 'Trakt', about: 'Trakt members who liked it' },
	{ id: 'tmdb', name: 'TMDB', about: 'TMDB members’ average, out of 10' }
];

export const DEFAULT_RATINGS: RatingSource[] = ['imdb', 'rt_critics', 'rt_audience'];
const ORDER = RATING_SOURCES.map((s) => s.id);
const KEY = 'holyarrs.ratings';

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

export const ratingPicks = $state<{ list: RatingSource[] }>({ list: typeof window === 'undefined' ? DEFAULT_RATINGS : stored() });

export function setRatingPick(id: RatingSource, on: boolean) {
	const next = ORDER.filter((s) => (s === id ? on : ratingPicks.list.includes(s)));
	ratingPicks.list = next;
	try {
		localStorage.setItem(KEY, JSON.stringify(next));
	} catch {
		/* Kept for this visit only. */
	}
}

export function resetRatingPicks() {
	ratingPicks.list = DEFAULT_RATINGS;
	try {
		localStorage.removeItem(KEY);
	} catch {
		/* Nothing kept. */
	}
}

/**
 * shownRatings is what a title shows: the picked outside scores it has and the
 * TMDB score when picked; with none of them, the first score it has in ORDER.
 */
export function shownRatings(scores: OutsideScore[], tmdb: number | null | undefined, picks: RatingSource[]): { scores: OutsideScore[]; tmdb: number | undefined } {
	const out = scores.filter((s) => picks.includes(s.source as RatingSource));
	const t = tmdb && picks.includes('tmdb') ? tmdb : undefined;
	if (out.length || t) return { scores: out, tmdb: t };
	for (const id of ORDER) {
		if (id === 'tmdb' && tmdb) return { scores: [], tmdb };
		const s = scores.find((x) => x.source === id);
		if (s) return { scores: [s], tmdb: undefined };
	}
	return { scores: [], tmdb: undefined };
}
