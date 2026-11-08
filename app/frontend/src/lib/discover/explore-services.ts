// The streaming services a person has, and the top lists ranked on them.

import { serviceRegion } from '#lib/ui/region.ts';
import { have } from './explore-rows.ts';

export { serviceRegion };
import { type ProviderItem } from '#lib/ui/title-cards.ts';
import { type ExploreType } from './explore-type.ts';

/** One streaming service from GET /watch-services. */
export interface WatchService {
	id: number;
	name: string;
	logo?: string;
	priority: number;
}

/** How many services the picker offers, the region's best known first. */
export const SERVICES_SHOWN = 6;

/** The services the picker shows: the region's top few plus any picked that fell out of it. */
export function pickerServices(all: WatchService[], picked: number[], n = SERVICES_SHOWN): WatchService[] {
	const top = [...all].sort((a, b) => a.priority - b.priority).slice(0, n);
	const extra = all.filter((s) => picked.includes(s.id) && !top.some((t) => t.id === s.id));
	return [...top, ...extra];
}

/** Adds or removes one service id, keeping the order picked. */
export function toggleService(picked: number[], id: number): number[] {
	return picked.includes(id) ? picked.filter((x) => x !== id) : [...picked, id];
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

export function writeServices(region: string, ids: number[], raw: string | null): string {
	let all: Record<string, number[]> = {};
	try {
		const v = JSON.parse(raw ?? 'null');
		if (v && typeof v === 'object' && !Array.isArray(v)) all = v;
	} catch {
		/* Start over. */
	}
	return JSON.stringify({ ...all, [region]: ids });
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

/** GET /explore/top for one chip. */
export function topPath(q: TopQuery): string {
	const p = new URLSearchParams({ type: q.type === 'music' ? 'all' : q.type, chip: String(q.chip), region: q.region });
	if (q.services.length) p.set('services', q.services.join('|'));
	if (q.genre && (q.type === 'movies' || q.type === 'shows')) p.set('genre', String(q.genre));
	return `/explore/top?${p.toString()}`;
}

/** All my services, then each picked service in the picker's order. */
export function topChips(picked: number[], shown: WatchService[]): { id: TopChip; name: string; logo?: string }[] {
	const byId = new Map(shown.map((s) => [s.id, s]));
	return [{ id: 'all' as TopChip, name: 'All my services' }, ...picked.filter((id) => byId.has(id)).map((id) => ({ id: id as TopChip, name: byId.get(id)!.name, logo: byId.get(id)!.logo }))];
}

/** The small line under the row title: where the list comes from, honestly named. */
export function topSource(a: Pick<TopAnswer, 'source' | 'region'>, serviceName: string): string {
	const where = a.region === 'global' ? 'worldwide' : a.region;
	return `Popular on ${serviceName} · ${where}`;
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
	if (r.source !== 'streaming-availability') return `Popular on ${r.name}`;
	if (r.age_seconds < 24 * 3600) return `Trending now on ${r.name}`;
	const d = new Date(r.fetched_at);
	const when = Number.isNaN(d.getTime()) ? 'recently' : d.toLocaleDateString('en-US', { month: 'short', day: 'numeric', timeZone: 'UTC' });
	return `Trending on ${r.name} as of ${when}`;
}
