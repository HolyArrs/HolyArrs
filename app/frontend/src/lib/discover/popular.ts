// New & Popular: paths and the small pure rules the page and Explore share.
import { cardIdentity, type ExploreCard, type ProviderItem } from '#lib/ui/title-cards.ts';
import { type ServerRow } from './explore-server.ts';
import { t, appLocale } from '#lib/core/i18n/i18n.svelte.ts';

export type NewType = 'all' | 'movies' | 'shows' | 'music';

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

export interface NewServiceAnswer {
	type: string;
	service: number;
	building: boolean;
	row: ServerRow;
}

/** A row needs this many cards to show. */
export const NEW_ROW_MIN = 6;

/** Reads ?type=; anything else is All. */
export function parseNewType(v: string | null): NewType {
	return v === 'movies' || v === 'shows' || v === 'music' ? v : 'all';
}

export function newPath(t: NewType, page: number): string {
	return `/explore/new?type=${t}&page=${page}`;
}

export function newServicesPath(t: string): string {
	return `/explore/new/services?type=${t === 'movies' || t === 'shows' ? t : 'all'}`;
}

/** The server's name for a chip row (services, hot_shows, hot_movies). */
export function serviceRowName(rowType: string): string {
	return rowType === 'np_top_shows' ? 'hot_shows' : rowType === 'np_top_movies' ? 'hot_movies' : 'services';
}

/** Asks for one service's own list on a chip row. */
export function newServicePath(t: NewType, service: number, rowType: string): string {
	return `/explore/new/service?id=${service}&type=${t}&row=${serviceRowName(rowType)}`;
}

/** The New & Popular link for an Explore tab (music has none, so All). */
export function newHref(t: string): string {
	return t === 'movies' || t === 'shows' ? `/popular?type=${t}` : '/popular';
}

/** Ranked rows draw their place number. */
export function isRanked(rowType: string): boolean {
	return rowType === 'np_top_shows' || rowType === 'np_top_movies';
}

/** Drops cards already shown elsewhere on the page (by cardIdentity or by a "kind:tmdb" key) and returns [] when fewer than `min` remain. */
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

/** The services a row's cards are on, in first-seen order. */
export function cardServices(cards: readonly Pick<ExploreCard, 'streaming'>[]): { service: number; name: string }[] {
	const out: { service: number; name: string }[] = [];
	for (const c of cards) {
		const s = c.streaming;
		if (s && !out.some((o) => o.service === s.service)) out.push({ service: s.service, name: s.name });
	}
	return out;
}

/** The rows that draw a chip per service (each service's own list). */
export function hasServiceChips(rowType: string): boolean {
	return rowType === 'np_services' || isRanked(rowType);
}

/** Keeps one service's cards (null means all); a ranked row keeps its order. */
export function pickService<C extends Pick<ExploreCard, 'streaming'>>(cards: readonly C[], service: number | null): C[] {
	return service === null ? [...cards] : cards.filter((c) => c.streaming?.service === service);
}

const DAY_MS = 86_400_000;

/** How many days `day` (YYYY-MM-DD) is from `today` (a local date). */
export function dayOffset(day: string, today: Date): number {
	const [y, m, d] = day.split('-').map(Number);
	const a = Date.UTC(y, m - 1, d);
	const b = Date.UTC(today.getFullYear(), today.getMonth(), today.getDate());
	return Math.round((a - b) / DAY_MS);
}

/** The rail's big day word and small date: Today, Yesterday, Tomorrow, else the weekday. */
export function railLabel(day: string, today: Date): { big: string; small: string } {
	const [y, m, d] = day.split('-').map(Number);
	const date = new Date(Date.UTC(y, m - 1, d));
	const off = dayOffset(day, today);
	const weekday = date.toLocaleDateString(appLocale('en-US'), { weekday: 'short', timeZone: 'UTC' });
	const small = date.toLocaleDateString(appLocale('en-US'), { month: 'short', day: 'numeric', timeZone: 'UTC' });
	const big = off === 0 ? t('discover.popular.today') : off === -1 ? t('discover.popular.yesterday') : off === 1 ? t('discover.popular.tomorrow') : weekday;
	return { big, small: off === 0 || off === -1 || off === 1 ? `${weekday} ${small}` : small };
}

/** A card's "kind:tmdb" key ("movie:123"). */
function plainKey(c: Pick<ExploreCard, 'kind' | 'ref'>): string {
	return c.ref?.startsWith('tmdb:') ? `${c.kind}:${c.ref.slice(5)}` : '';
}
