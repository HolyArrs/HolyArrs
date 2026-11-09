// The type switch (All, Movies, Shows, Music) and the genre rows that follow it.

import { type CardKind, type ExploreRow } from '#lib/ui/title-cards.ts';
import { keepFull } from './explore-rows.ts';

export type ExploreType = 'all' | 'movies' | 'shows' | 'music';

/** One card of Browse by genre (GET /explore/genre-row). */
export interface GenreRowCard {
	kind: 'movie' | 'show';
	id: number;
	name: string;
	backdrop: string;
	from: string;
	/** The genre page holds movies and shows (opened under All). */
	both?: boolean;
}

const typeKind: Record<Exclude<ExploreType, 'all'>, CardKind> = { movies: 'movie', shows: 'show', music: 'artist' };

/** The shelves that hold one kind only, and the type they belong to. */
const kindRows: Record<string, Exclude<ExploreType, 'all'>> = { movies: 'movies', shows: 'shows', music: 'music' };

/** Reads ?type=; anything unknown (or music while it is off) is All. */
export function parseType(v: string | null | undefined, musicOn = true): ExploreType {
	if (v === 'movies' || v === 'shows' || (v === 'music' && musicOn)) return v;
	return 'all';
}

/** The card kind a type keeps, or null for All. */
export function typeKindOf(t: ExploreType): CardKind | null {
	return t === 'all' ? null : typeKind[t];
}

/** Keeps the cards of one type in every row, drops the kind shelves of other types and hides rows left empty. All keeps everything. */
export function filterRows(rows: ExploreRow[], t: ExploreType): ExploreRow[] {
	const kind = typeKindOf(t);
	if (!kind) return rows;
	return keepFull(rows.filter((r) => !kindRows[r.id] || kindRows[r.id] === t).map((r) => ({ ...r, cards: r.cards.filter((c) => c.kind === kind) })));
}

/** A row with fewer cards than this is left out; rows kept on purpose, like Trending on streaming, stay. */
export const ROW_MIN = 8;

/** Drops rows under `min` cards, keeping rows marked keep. */
export function enoughCards<T extends { cards: readonly unknown[]; keep?: boolean }>(rows: T[], min = ROW_MIN): T[] {
	return rows.filter((r) => r.keep || r.cards.length >= min);
}

export const GENRE_ROW_MAX = 16;

/** GET /explore/genre-row for a tab. */
export function genreRowPath(t: ExploreType): string {
	return `/explore/genre-row?${new URLSearchParams({ type: t === 'music' ? 'all' : t }).toString()}`;
}

/** Opens the same genre page as the Genres picker. */
export function genreCardHref(g: Pick<GenreRowCard, 'kind' | 'id' | 'both'>): string {
	return g.both ? `/explore?genre=${g.id}` : `/explore?type=${g.kind === 'show' ? 'shows' : 'movies'}&genre=${g.id}`;
}

/** The row index Browse by genre follows: row 6 (index 5) or the first later spot not next to a genre row; -1 with fewer than 6 rows. */
export function genreRowAfter(rows: readonly { type?: string }[]): number {
	const genreish = (r?: { type?: string }) => !!r?.type && r.type.includes('genre');
	for (let i = 5; i < rows.length; i++) if (!genreish(rows[i]) && !genreish(rows[i + 1])) return i;
	return -1;
}

/** Carries the type to a "See all" target: kind= for the Library, type= elsewhere. */
export function typeHref(href: string | undefined, t: ExploreType): string | undefined {
	const kind = typeKindOf(t);
	if (!href || !kind) return href;
	const [path, query = ''] = href.split('?');
	const p = new URLSearchParams(query);
	if (/\/library\/?$/.test(path)) {
		if (p.has('kind') && p.get('kind') !== kind) return href;
		p.set('kind', kind);
	} else p.set('type', t);
	return `${path}?${p.toString()}`;
}
