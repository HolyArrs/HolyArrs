// New & Popular: paths and the small pure rules the page and Explore share.
import { cardIdentity, type ExploreCard, type ProviderItem, type ServerRow } from './explore.ts';

export type NewType = 'all' | 'movies' | 'shows';

/** A row needs this many cards to show. */
export const NEW_ROW_MIN = 6;

export interface NewAnswer {
	type: string;
	page: number;
	more: boolean;
	building: boolean;
	/** Whether the member picked their services (page 0). */
	services_set?: boolean;
	/** This week (page 0 only): days -3 to +7. */
	rail?: RailDay[];
	/** The hero (page 0 only): up to 6 new titles with a trailer, logo and backdrop. */
	hero?: ProviderItem[];
	rows: ServerRow[];
}

/** One title on the This week rail: what happens that day, and the bell. */
export type RailItem = ProviderItem & { what: string; following: boolean; upcoming: boolean };

export interface RailDay {
	day: string;
	items: RailItem[];
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

/** chipRows are the rows that draw a chip per service (each service's own list). */
export function hasServiceChips(rowType: string): boolean {
	return rowType === 'np_services' || isRanked(rowType);
}

/** pickService keeps one service's cards (null: all); a ranked row keeps its order. */
export function pickService<C extends Pick<ExploreCard, 'streaming'>>(cards: readonly C[], service: number | null): C[] {
	return service === null ? [...cards] : cards.filter((c) => c.streaming?.service === service);
}

const DAY_MS = 86_400_000;

/** dayOffset is how many days `day` (YYYY-MM-DD) is from `today` (a local date). */
export function dayOffset(day: string, today: Date): number {
	const [y, m, d] = day.split('-').map(Number);
	const a = Date.UTC(y, m - 1, d);
	const b = Date.UTC(today.getFullYear(), today.getMonth(), today.getDate());
	return Math.round((a - b) / DAY_MS);
}

/** railLabel is the rail's big day word and its small date: Today, Yesterday, Tomorrow, else the weekday. */
export function railLabel(day: string, today: Date): { big: string; small: string } {
	const [y, m, d] = day.split('-').map(Number);
	const date = new Date(Date.UTC(y, m - 1, d));
	const off = dayOffset(day, today);
	const weekday = date.toLocaleDateString('en-US', { weekday: 'short', timeZone: 'UTC' });
	const small = date.toLocaleDateString('en-US', { month: 'short', day: 'numeric', timeZone: 'UTC' });
	const big = off === 0 ? 'Today' : off === -1 ? 'Yesterday' : off === 1 ? 'Tomorrow' : weekday;
	return { big, small: off === 0 || off === -1 || off === 1 ? `${weekday} ${small}` : small };
}

/** railBell says what a rail title shows on the right: the Follow bell, "Coming" (in the Library already), or nothing. */
export function railBell(it: Pick<RailItem, 'upcoming' | 'in_library' | 'kind'>): 'bell' | 'coming' | null {
	if (!it.upcoming) return null;
	if (it.in_library) return 'coming';
	return it.kind === 'movie' || it.kind === 'show' ? 'bell' : null;
}