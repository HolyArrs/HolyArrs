// Person page types and helpers for stats chips, decade groups and bulk add or ask.

import { t } from '#lib/core/i18n/i18n.svelte.ts';
export type PersonRole = 'all' | 'actor' | 'director';

export type PersonSummary = {
	id: string;
	name: string;
	department?: string;
	profile?: string;
	known_for: PersonMovie[];
};

export type Person = PersonSummary & {
	biography?: string;
	birthday?: string;
	deathday?: string;
	birthplace?: string;
};

export type PersonMovie = {
	name: string;
	year: number;
	overview: string;
	poster?: string;
	backdrop?: string;
	cinema_date?: string;
	popularity?: number;
	ids: Record<string, string>;
	roles?: string[];
	characters?: string[];
	title_id?: number;
	never_add?: boolean;
	ask?: { id: number; state: string; state_label: string; mine: boolean; people: number };
};

export type FilmographyPage = {
	person: Person;
	movies: PersonMovie[];
	page: number;
	page_size: number;
	total_pages: number;
	total_results: number;
	complete: boolean;
	provider_pages: number;
	library?: PersonMovie[];
	asked?: PersonMovie[];
	known_for?: PersonMovie[];
	coming_soon?: PersonMovie[];
	often_with?: PersonSummary[];
	stats?: PersonStats;
	attribution: string;
};

export type PersonStats = { first_year?: number; last_year?: number; active: boolean; films: number; shows?: number; titles?: number; rating?: number; rated?: number };

export type FilmSort = 'oldest' | 'newest' | 'popular';

export type ReviewedResult = { tmdb_id: string; ok: boolean; error?: string };

// Short facts under the name; the dash range ends at 'now' while still active.
export function personChips(s: PersonStats | undefined, library: number, asked: number): string[] {
	if (!s) return [];
	const out: string[] = [];
	if (s.first_year) out.push(`${s.first_year}–${s.active ? t('ui.people.now') : s.last_year}`);
	const films = s.films ? (s.films === 1 ? t('ui.people.filmOne') : t('ui.people.filmOther', { count: s.films })) : '';
	if (s.shows) {
		const titles = s.titles || s.films + s.shows;
		out.push([titles === 1 ? t('ui.people.titleOne') : t('ui.people.titleOther', { count: titles }), films, s.shows === 1 ? t('ui.people.showOne') : t('ui.people.showOther', { count: s.shows })].filter(Boolean).join(' · '));
	} else if (films) out.push(films);
	if (s.rating && s.rated) out.push(t('ui.people.average', { rating: s.rating.toFixed(1) }));
	const home = [library ? t('ui.people.inLibrary', { count: library }) : '', asked ? t('ui.people.asked', { count: asked }) : ''].filter(Boolean).join(' · ');
	if (home) out.push(home);
	return out;
}

// Groups keep first-seen order; undated films go to 'Coming soon'.
export function byDecade(movies: PersonMovie[]): { label: string; movies: PersonMovie[] }[] {
	const groups: { label: string; movies: PersonMovie[] }[] = [];
	const at = new Map<string, number>();
	for (const m of movies) {
		const label = m.year ? `${Math.floor(m.year / 10) * 10}s` : t('ui.people.comingSoon');
		let i = at.get(label);
		if (i === undefined) {
			i = groups.push({ label, movies: [] }) - 1;
			at.set(label, i);
		}
		groups[i].movies.push(m);
	}
	return groups;
}

// Keeps only plain numeric ids (max 12 digits) and drops repeats before posting.
export function normalizeSelection(ids: string[]): string[] {
	const seen = new Set<string>();
	const out: string[] = [];
	for (const raw of ids) {
		const id = raw.trim();
		if (!/^\d{1,12}$/.test(id) || seen.has(id)) continue;
		seen.add(id);
		out.push(id);
	}
	return out;
}

// Posts one at a time so one failure does not stop the rest.
export async function runReviewed(
	post: (path: string, body: Record<string, unknown>) => Promise<unknown>,
	action: 'add' | 'ask',
	ids: string[]
): Promise<ReviewedResult[]> {
	const results: ReviewedResult[] = [];
	for (const tmdb_id of normalizeSelection(ids)) {
		try {
			await post(action === 'add' ? '/titles' : '/asks', { id: `tmdb:${tmdb_id}` });
			results.push({ tmdb_id, ok: true });
		} catch (err) {
			results.push({ tmdb_id, ok: false, error: errorText(err) });
		}
	}
	return results;
}

// Fallback text so the user never sees an empty error.
function errorText(err: unknown): string {
	if (err instanceof Error && err.message.trim()) return err.message;
	return t('ui.people.couldNotSubmit');
}
