// The grown card's two text lines under the title: the facts (year, length,
// age rating, stars), then one context line (the household's status, else
// why the card is shown). Pure, so the card and its tests share them.

import type { Fresh } from './fresh.ts';

/** "1h 46m" from minutes. */
export function shortRuntime(min?: number): string {
	if (!min || min <= 0) return '';
	const h = Math.floor(min / 60);
	const m = Math.round(min % 60);
	return h ? (m ? `${h}h ${m}m` : `${h}h`) : `${m}m`;
}

export interface FactsIn {
	kind: 'movie' | 'show';
	year?: number;
	/** Shows: how many seasons (specials not counted). */
	seasons?: number;
	/** Movies: minutes. */
	runtime?: number;
	certification?: string;
	/** Already passed through shownScore (undefined under 25 votes). */
	score?: number;
}

export type Fact = { text: string; part: 'year' | 'length' | 'rating' | 'stars' };

/** growFacts: shows "2024 · 3 seasons · TV-PG · ★ 6.8", movies "2024 · 1h 46m · PG-13 · ★ 6.6"; missing parts are left out. */
export function growFacts(f: FactsIn): Fact[] {
	const out: Fact[] = [];
	if (f.year) out.push({ text: String(f.year), part: 'year' });
	const length = f.kind === 'show' ? (f.seasons ? `${f.seasons} season${f.seasons === 1 ? '' : 's'}` : '') : shortRuntime(f.runtime);
	if (length) out.push({ text: length, part: 'length' });
	if (f.certification) out.push({ text: f.certification, part: 'rating' });
	if (f.score) out.push({ text: `★ ${(Math.round(f.score * 10) / 10).toFixed(1)}`, part: 'stars' });
	return out;
}

/** Genres that carry no mood: a mood word on them is not trusted. */
const PLAIN = new Set(['Reality', 'Talk', 'News', 'Soap']);
/** The moods a genre plainly rules out. */
const CLASH: Record<string, string[]> = {
	Scary: ['Family', 'Kids', 'Animation', 'Comedy'],
	Cozy: ['Horror', 'War', 'Crime', 'Thriller'],
	'Feel-good': ['Horror', 'War', 'Thriller'],
	Gritty: ['Family', 'Kids', 'Animation'],
	Romantic: ['Horror', 'Documentary']
};
const MOODS = new Set(['Dark', 'Exciting', 'Feel-good', 'Suspenseful', 'Witty', 'Romantic', 'Emotional', 'Mind-bending', 'Cozy', 'Gritty', 'Inspiring', 'Scary']);

/**
 * fitTags: two or three words the title's data supports. Moods that the
 * genres contradict (or any mood on a Reality, Talk or News title) are
 * dropped; the genres top the list up to two.
 */
export function fitTags(moods: readonly string[] | undefined, genres: readonly string[] | undefined, max = 3): string[] {
	const g = genres ?? [];
	const plain = g.some((x) => PLAIN.has(x));
	const out: string[] = [];
	for (const m of moods ?? []) {
		if (MOODS.has(m) && (plain || (CLASH[m] ?? []).some((x) => g.includes(x)))) continue;
		if (!out.includes(m)) out.push(m);
	}
	for (const x of g) {
		if (out.length >= 2) break;
		if (!out.includes(x)) out.push(x);
	}
	return out.slice(0, max);
}

/** The household's status for the context line (from the card's owned mark). */
export interface HouseIn {
	state?: string;
	/** The mark's own words, such as "Downloading 42%". */
	label?: string;
	/** The signed-in person asked for it. */
	mine?: boolean;
	/** Names of others who asked, as this member may see them. */
	askers?: string[];
	fresh?: Fresh | null;
}

/** houseLine: "In your Library · up to date", "Downloading 42%", "Asked by Mia", "Coming Nov 14", or ''. */
export function houseLine(h: HouseIn): string {
	switch (h.state) {
		case 'available':
			return 'In your Library · up to date';
		case 'partial':
			return 'In your Library · some missing';
		case 'downloading':
			return h.label && /\d/.test(h.label) ? h.label : 'Downloading';
		case 'requested':
			if (h.mine) return 'You asked for it';
			if (h.askers?.length) return `Asked by ${h.askers.length > 2 ? `${h.askers[0]} and ${h.askers.length - 1} others` : h.askers.join(' and ')}`;
			return h.label || 'Asked for';
	}
	if (h.fresh && (h.fresh.kind === 'coming' || h.fresh.kind === 'season_coming') && h.fresh.text) return h.fresh.text;
	return '';
}

export interface WhyIn {
	rowType?: string;
	rowTitle?: string;
	fresh?: Fresh | null;
	streaming?: { name: string; rank?: number } | null;
}

/** Row types whose title is itself the reason (Because you have Slow Horses). */
const SEEDED = new Set(['because_have', 'because_asked', 'person', 'household']);

/** whyShown: "#3 on Netflix today", "Because you have Slow Horses", "New season", or ''. */
export function whyShown(w: WhyIn): string {
	if (w.streaming?.rank) return `#${w.streaming.rank} on ${w.streaming.name} today`;
	if (w.rowType && SEEDED.has(w.rowType) && w.rowTitle) return w.rowTitle;
	if (w.fresh?.text) return w.fresh.text;
	return '';
}

/** contextLine: the household status when there is one, else why the card is shown. */
export function contextLine(h: HouseIn, w: WhyIn): { text: string; kind: 'house' | 'why' } | null {
	const house = houseLine(h);
	if (house) return { text: house, kind: 'house' };
	const why = whyShown(w);
	return why ? { text: why, kind: 'why' } : null;
}
