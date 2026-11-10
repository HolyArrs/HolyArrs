// The streaming services a person has, and the top lists ranked on them.

import { serviceRegion } from '#lib/ui/region.ts';
import { have } from './explore-rows.ts';
import { type ProviderItem } from '#lib/ui/title-cards.ts';
import { type ExploreType } from './explore-type.ts';
import { t, appLocale } from '#lib/core/i18n/i18n.svelte.ts';

/** One streaming service from GET /watch-services. */
export interface WatchService {
	id: number;
	name: string;
	logo?: string;
	priority: number;
}

/** A chip of the Top on your services row: all, or one service id. */
export type TopChip = 'all' | number;

export interface TopQuery {
	type: ExploreType;
	chip: TopChip;
	services: number[];
	region: string;
	genre?: number;
}

export interface TopAnswer {
	chip: string;
	region: string;
	/** TMDB's most popular this week on the service. */
	source: 'popular';
	items: (ProviderItem & { rank: number })[];
}

/** One streaming service's list from GET /explore/service-top. */
export interface ServiceTopRow {
	/** The TMDB watch provider id (the picker's ids). */
	service: number;
	key: string;
	name: string;
	/** streaming-availability: the service's own top today. tmdb-popular: TMDB's most popular on it. */
	source: 'streaming-availability' | 'tmdb-popular';
	fetched_at: string;
	age_seconds: number;
	stale: boolean;
	/** Only the service's own top list is a ranking. */
	ranked: boolean;
	items: (ProviderItem & { rank: number })[];
}

export interface ServiceTopAnswer {
	region: string;
	/** Must be shown with the lists. */
	attribution: string;
	services: ServiceTopRow[];
}

export { serviceRegion };

/** How many services the picker offers, the region's best known first. */
export const SERVICES_SHOWN = 6;

/** The services the picker shows: the region's top few plus any picked that fell out of it. */
export function pickerServices(all: WatchService[], picked: number[], n = SERVICES_SHOWN): WatchService[] {
	const top = [...all].sort((a, b) => a.priority - b.priority).slice(0, n);
	const extra = all.filter((s) => picked.includes(s.id) && !top.some((t) => t.id === s.id));
	return [...top, ...extra];
}

/** Saved choice as { region: [ids] }; null when never picked, so the first-time prompt shows. */
export const SERVICES_KEY = 'holyarrs.explore.services';

export function readServices(region: string, raw: string | null): number[] | null {
	try {
		const v = JSON.parse(raw ?? 'null')?.[region];
		return Array.isArray(v) ? v.filter((x): x is number => Number.isInteger(x) && x > 0) : null;
	} catch {
		return null;
	}
}

/** GET /explore/top for one chip. */
export function topPath(q: TopQuery): string {
	const p = new URLSearchParams({ type: q.type === 'music' ? 'all' : q.type, chip: String(q.chip), region: q.region });
	if (q.services.length) p.set('services', q.services.join('|'));
	if (q.genre && (q.type === 'movies' || q.type === 'shows')) p.set('genre', String(q.genre));
	return `/explore/top?${p.toString()}`;
}

/** GET /explore/service-top: every service at once. */
export function serviceTopPath(t: ExploreType, region: string): string {
	return `/explore/service-top?${new URLSearchParams({ type: t === 'music' ? 'all' : t, region }).toString()}`;
}

/** The services with a list, the person's own first (in their order), then the rest. */
export function serviceChips(rows: ServiceTopRow[], mine: number[]): ServiceTopRow[] {
	const have = rows.filter((r) => r.items.length > 0);
	const pos = (r: ServiceTopRow) => {
		const i = mine.indexOf(r.service);
		return i < 0 ? mine.length + have.indexOf(r) : i;
	};
	return [...have].sort((a, b) => pos(a) - pos(b));
}

/** The line under the row title for one service's list. */
export function serviceLabel(r: Pick<ServiceTopRow, 'source' | 'name' | 'age_seconds' | 'fetched_at'>): string {
	if (r.source !== 'streaming-availability') return t('discover.exploreServices.popularOn', { name: r.name });
	if (r.age_seconds < 24 * 3600) return t('discover.exploreServices.trendingNowOn', { name: r.name });
	const d = new Date(r.fetched_at);
	const when = Number.isNaN(d.getTime()) ? t('discover.exploreServices.recently') : d.toLocaleDateString(appLocale('en-US'), { month: 'short', day: 'numeric', timeZone: 'UTC' });
	return t('discover.exploreServices.trendingOnAsOf', { name: r.name, when });
}
