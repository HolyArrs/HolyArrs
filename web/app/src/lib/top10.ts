// Top 10 on Netflix: the metadata server's weekly lists as a poster row.
import { api, ApiError, type Client } from './api.ts';
import { discoverCard, type ExploreCard, type ProviderItem } from './explore.ts';

export type Top10Kind = 'movie' | 'show';

export interface Top10Item extends ProviderItem {
	rank: number;
	season_title?: string;
}

export interface Top10Answer {
	week: string;
	region: string;
	regions: string[];
	items: Top10Item[];
}

export interface Top10Card extends ExploreCard {
	rank: number;
}

/** top10Cards turns the answer into poster cards that keep Netflix's rank. */
export function top10Cards(a: Top10Answer): Top10Card[] {
	return a.items.map((d) => {
		const season = d.season_title?.startsWith(`${d.name}: `) ? d.season_title.slice(d.name.length + 2) : d.season_title;
		return { ...discoverCard(d, 'top10'), rank: d.rank, subtitle: season || undefined };
	});
}

/** defaultRegion is the country of the first browser language that names one, else global. */
export function defaultRegion(langs: readonly string[] = typeof navigator === 'undefined' ? [] : navigator.languages): string {
	for (const l of langs) {
		const m = /^[a-z]{2,3}-([A-Za-z]{2})$/.exec(l);
		if (m) return m[1].toUpperCase();
	}
	return 'global';
}

/** fetchTop10 reads one list; a country without one falls back to global; null when there is none. */
export async function fetchTop10(kind: Top10Kind, region: string, client: Pick<Client, 'get'> = api, signal?: AbortSignal): Promise<Top10Answer | null> {
	for (const r of region === 'global' ? ['global'] : [region, 'global']) {
		try {
			const a = await client.get<Top10Answer>(`/top10?kind=${kind}&region=${encodeURIComponent(r)}`, signal);
			return a.items.length ? a : null;
		} catch (e) {
			if (!(e instanceof ApiError && e.status === 404)) return null;
		}
	}
	return null;
}
