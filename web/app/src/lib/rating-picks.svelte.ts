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

let saved = false;
try {
	saved = typeof window !== 'undefined' && localStorage.getItem(KEY) !== null;
} catch {
	/* No storage: the defaults. */
}
// custom: picked here at least once; while false a title fills up to three scores.
export const ratingPicks = $state<{ list: RatingSource[]; custom: boolean }>({ list: typeof window === 'undefined' ? DEFAULT_RATINGS : stored(), custom: saved });

export function setRatingPick(id: RatingSource, on: boolean) {
	const next = ORDER.filter((s) => (s === id ? on : ratingPicks.list.includes(s)));
	ratingPicks.list = next;
	ratingPicks.custom = true;
	try {
		localStorage.setItem(KEY, JSON.stringify(next));
	} catch {
		/* Kept for this visit only. */
	}
}

export function resetRatingPicks() {
	ratingPicks.list = DEFAULT_RATINGS;
	ratingPicks.custom = false;
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
export function shownRatings(scores: OutsideScore[], tmdb: number | null | undefined, picks: RatingSource[], fill = false): { scores: OutsideScore[]; tmdb: number | undefined } {
	const out = scores.filter((s) => picks.includes(s.source as RatingSource));
	let t = tmdb && picks.includes('tmdb') ? tmdb : undefined;
	// Still on the default three (never changed here): a title missing one of
	// them fills up to three with the next scores it has, TMDB last.
	if (fill) {
		for (const s of scores.filter((x) => !out.includes(x)).sort((a, b) => ORDER.indexOf(a.source as RatingSource) - ORDER.indexOf(b.source as RatingSource))) {
			if (out.length >= 3) break;
			out.push(s);
		}
		if (out.length < 3 && tmdb && !t) t = tmdb;
	}
	if (out.length || t) return { scores: out, tmdb: t };
	for (const id of ORDER) {
		if (id === 'tmdb' && tmdb) return { scores: [], tmdb };
		const s = scores.find((x) => x.source === id);
		if (s) return { scores: [s], tmdb: undefined };
	}
	return { scores: [], tmdb: undefined };
}
