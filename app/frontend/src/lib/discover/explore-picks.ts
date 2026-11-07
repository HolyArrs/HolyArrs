// Explore rows from the discover endpoint: trending, best, upcoming, similar, genre.

import { api, ApiError, type Client } from '../core/api.ts';
import { type ProviderItem } from '../ui/title-cards.ts';

export type ProviderRowName = 'trending' | 'best' | 'upcoming' | 'similar' | 'genre' | 'services';

export interface ProviderRowQuery {
	row: ProviderRowName;
	/** similar: the Library title id. */
	id?: number;
	/** genre: the genre name. */
	g?: string;
	kind?: 'movie' | 'show';
	/** services: streaming service ids. */
	services?: number[];
	/** services: the two-letter country. */
	region?: string;
}

export interface ProviderRowAnswer {
	row: ProviderRowName;
	title?: string;
	items: ProviderItem[];
}

export function discoverPath(q: ProviderRowQuery): string {
	const p = new URLSearchParams({ row: q.row });
	if (q.id !== undefined) p.set('id', String(q.id));
	if (q.g) p.set('g', q.g);
	if (q.kind) p.set('kind', q.kind);
	if (q.services?.length) p.set('services', q.services.join('|'));
	if (q.region) p.set('region', q.region);
	return `/discover?${p.toString()}`;
}

/** fetchProviderRow asks the server for a discover row; null when the server has none yet (404/501). */
export async function fetchProviderRow(q: ProviderRowQuery, client: Pick<Client, 'get'> = api, signal?: AbortSignal): Promise<ProviderRowAnswer | null> {
	try {
		return await client.get<ProviderRowAnswer>(discoverPath(q), signal);
	} catch (e) {
		if (e instanceof ApiError && (e.status === 404 || e.status === 501)) return null;
		throw e;
	}
}

export const discoverTitles: Record<Exclude<ProviderRowName, 'similar' | 'genre'>, string> = {
	trending: 'Trending this week everywhere',
	best: 'Best new releases',
	services: 'On your services',
	upcoming: 'Coming to cinemas'
};
