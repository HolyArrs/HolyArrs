// New & Popular: paths and the small pure rules the page and Explore share.
import { cardIdentity, type ExploreCard, type ServerRow } from './explore.ts';

export type NewType = 'all' | 'movies' | 'shows';

/** A row needs this many cards to show. */
export const NEW_ROW_MIN = 6;

export interface NewAnswer {
	type: string;
	page: number;
	more: boolean;
	building: boolean;
	rows: ServerRow[];
}

export interface NewServicesAnswer {
	type: string;
	row: ServerRow;
}

/** parseNewType reads ?type= (anything else is All). */
export function parseNewType(v: string | null): NewType {
	return v === 'movies' || v === 'shows' ? v : 'all';
}

export function newPath(t: NewType, page: number): string {
	return `/explore/new?type=${t}&page=${page}`;
}

export function newServicesPath(t: string): string {
	return `/explore/new/services?type=${t === 'movies' || t === 'shows' ? t : 'all'}`;
}

/** The New & Popular link for an Explore tab (music has none: All). */
export function newHref(t: string): string {
	return t === 'movies' || t === 'shows' ? `/new?type=${t}` : '/new';
}

/** Ranked rows draw their place number. */
export function isRanked(rowType: string): boolean {
	return rowType === 'np_top_shows' || rowType === 'np_top_movies';
}

/** The key Explore's Top 10 uses ("movie:123"), from a card. */
function plainKey(c: Pick<ExploreCard, 'kind' | 'ref'>): string {
	return c.ref?.startsWith('tmdb:') ? `${c.kind}:${c.ref.slice(5)}` : '';
}

/**
 * leaveOut drops cards already shown elsewhere on the page (by cardIdentity
 * or by Top 10's "kind:tmdb" key) and returns [] when fewer than min remain.
 */
export function leaveOut<C extends Pick<ExploreCard, 'key' | 'kind' | 'ref' | 'titleId'>>(cards: readonly C[], taken: Iterable<string>, min = NEW_ROW_MIN): C[] {
	const seen = new Set(taken);
	const out: C[] = [];
	for (const c of cards) {
		const id = cardIdentity(c);
		const k = plainKey(c);
		if (seen.has(id) || (k && seen.has(k))) continue;
		seen.add(id);
		out.push(c);
	}
	return out.length >= min ? out : [];
}

/** serviceChips lists the services a row's cards are on, in first-seen order. */
export function cardServices(cards: readonly Pick<ExploreCard, 'streaming'>[]): { service: number; name: string }[] {
	const out: { service: number; name: string }[] = [];
	for (const c of cards) {
		const s = c.streaming;
		if (s && !out.some((o) => o.service === s.service)) out.push({ service: s.service, name: s.name });
	}
	return out;
}
