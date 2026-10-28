// Title details (backdrop, story, runtime, rating) for asks shown big on the
// Approvals looks. Asked once per title while the app is open, from the
// install's own metadata endpoint (the same answer the title page uses).
// The same answer also gives the seasons and release dates the Inbox
// summary reads, and a season's episodes are asked once when a pill opens
// or a recent season may still be airing.

import { api } from '../api.ts';
import { fromMetadata, type TitleDetails } from '../title-details.ts';
import type { Ask } from '../types.ts';
import { detailKey } from './approvals.ts';
import { readInfo, type AskInfo, type EpisodeInfo } from './inbox.ts';

const got = $state<Record<string, TitleDetails | null>>({});
const info = $state<Record<string, AskInfo | null>>({});
const eps = $state<Record<string, EpisodeInfo[] | null>>({});
const asked = new Set<string>();
const askedEps = new Set<string>();

/** askDetails is what is known for an ask: details, null (none, or not yet), undefined (music or no id). */
export function askDetails(a: Pick<Ask, 'kind' | 'tmdb_id'>): TitleDetails | null | undefined {
	const k = detailKey(a);
	return k ? (got[k] ?? null) : undefined;
}

/** askInfo is the seasons and release dates for an ask's summary (null until known or for music). */
export function askInfo(a: Pick<Ask, 'kind' | 'tmdb_id'>): AskInfo | null {
	const k = detailKey(a);
	return k ? (info[k] ?? null) : null;
}

/** wantDetails asks for an ask's details once. */
export function wantDetails(a: Pick<Ask, 'kind' | 'tmdb_id'>): void {
	const k = detailKey(a);
	if (!k || asked.has(k)) return;
	asked.add(k);
	const kind = a.kind === 'show' ? 'show' : 'movie';
	api
		.get<Record<string, unknown>>(`/metadata/${kind}/${encodeURIComponent(a.tmdb_id)}`)
		.then((raw) => {
			got[k] = fromMetadata(kind, a.tmdb_id, raw);
			info[k] = readInfo(raw);
		})
		.catch(() => {
			got[k] = null;
			info[k] = null;
		});
}

/** seasonEpisodes is one season's episodes (null until known), asked once. */
export function seasonEpisodes(tmdb: string, season: number): EpisodeInfo[] | null {
	const k = `${tmdb}:${season}`;
	if (!askedEps.has(k) && tmdb && season > 0) {
		askedEps.add(k);
		api
			.get<{ episodes?: EpisodeInfo[] }>(`/metadata/show/${encodeURIComponent(tmdb)}/seasons/${season}`)
			.then((r) => (eps[k] = Array.isArray(r.episodes) ? r.episodes : []))
			.catch(() => (eps[k] = []));
	}
	return eps[k] ?? null;
}
