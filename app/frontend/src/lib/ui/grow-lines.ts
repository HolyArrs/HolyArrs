
import type { Fresh } from './fresh.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';

// Text lines shown on the grown card: facts, tags, house state and why.

export interface FactsIn {
	kind: 'movie' | 'show';
	year?: number;
	seasons?: number;
	runtime?: number;
	certification?: string;
	score?: number;
}

export type Fact = { text: string; part: 'year' | 'length' | 'rating' | 'stars' };

export interface HouseIn {
	state?: string;
	label?: string;
	mine?: boolean;
	askers?: string[];
	fresh?: Fresh | null;
}

export interface WhyIn {
	rowType?: string;
	rowTitle?: string;
	fresh?: Fresh | null;
	streaming?: { name: string; rank?: number } | null;
}

// Streaming rank, then row reason, then freshness.
export function whyShown(w: WhyIn): string {
	if (w.streaming?.rank) return t('ui.growLines.rankedOn', { rank: w.streaming.rank, name: w.streaming.name });
	if (w.rowType && SEEDED.has(w.rowType) && w.rowTitle) return w.rowTitle;
	if (w.fresh?.text) return w.fresh.text;
	return '';
}


// Moods that clash with a genre read wrong, so they are dropped.
const PLAIN = new Set(['Reality', 'Talk', 'News', 'Soap']);
const CLASH: Record<string, string[]> = {
	Scary: ['Family', 'Kids', 'Animation', 'Comedy'],
	Cozy: ['Horror', 'War', 'Crime', 'Thriller'],
	'Feel-good': ['Horror', 'War', 'Thriller'],
	Gritty: ['Family', 'Kids', 'Animation'],
	Romantic: ['Horror', 'Documentary']
};
const MOODS = new Set(['Dark', 'Exciting', 'Feel-good', 'Suspenseful', 'Witty', 'Romantic', 'Emotional', 'Mind-bending', 'Cozy', 'Gritty', 'Inspiring', 'Scary']);


const SEEDED = new Set(['because_have', 'because_asked', 'person', 'household']);


export function shortRuntime(min?: number): string {
	if (!min || min <= 0) return '';
	const h = Math.floor(min / 60);
	const m = Math.round(min % 60);
	return h ? (m ? t('ui.growLines.hoursMinutes', { h, m }) : t('ui.growLines.hours', { h })) : t('ui.growLines.minutes', { m });
}

export function growFacts(f: FactsIn): Fact[] {
	const out: Fact[] = [];
	if (f.year) out.push({ text: String(f.year), part: 'year' });
	const length = f.kind === 'show' ? (f.seasons ? (f.seasons === 1 ? t('ui.growLines.seasonOne') : t('ui.growLines.seasonOther', { count: f.seasons })) : '') : shortRuntime(f.runtime);
	if (length) out.push({ text: length, part: 'length' });
	if (f.certification) out.push({ text: f.certification, part: 'rating' });
	if (f.score) out.push({ text: `★ ${(Math.round(f.score * 10) / 10).toFixed(1)}`, part: 'stars' });
	return out;
}

// Fills up to two tags from genres after moods; plain genres get no moods.
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

// Library state wins over every other line.
export function houseLine(h: HouseIn): string {
	switch (h.state) {
		case 'available':
			return t('ui.growLines.upToDate');
		case 'partial':
			return t('ui.growLines.someMissing');
		case 'downloading':
			return h.label && /\d/.test(h.label) ? h.label : t('ui.growLines.downloading');
		case 'requested':
			if (h.mine) return t('ui.growLines.youAsked');
			if (h.askers?.length) return h.askers.length > 2 ? t('ui.growLines.askedByMany', { name: h.askers[0], count: h.askers.length - 1 }) : h.askers.length === 2 ? t('ui.growLines.askedByTwo', { a: h.askers[0], b: h.askers[1] }) : t('ui.growLines.askedByOne', { name: h.askers[0] });
			return h.label || t('ui.growLines.askedFor');
	}
	if (h.fresh && (h.fresh.kind === 'coming' || h.fresh.kind === 'season_coming') && h.fresh.text) return h.fresh.text;
	return '';
}

export function contextLine(h: HouseIn, w: WhyIn): { text: string; kind: 'house' | 'why' } | null {
	const house = houseLine(h);
	if (house) return { text: house, kind: 'house' };
	const why = whyShown(w);
	return why ? { text: why, kind: 'why' } : null;
}
