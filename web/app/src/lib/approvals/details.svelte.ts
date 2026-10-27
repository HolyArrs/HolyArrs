// Title details (backdrop, story, runtime, rating) for asks shown big on the
// Approvals looks. Asked once per title while the app is open, from the
// install's own metadata endpoint (the same answer the title page uses).

import { api } from '../api.ts';
import { fromMetadata, type TitleDetails } from '../title-details.ts';
import type { Ask } from '../types.ts';
import { detailKey } from './approvals.ts';

const got = $state<Record<string, TitleDetails | null>>({});
const asked = new Set<string>();

/** askDetails is what is known for an ask: details, null (none, or not yet), undefined (music or no id). */
export function askDetails(a: Pick<Ask, 'kind' | 'tmdb_id'>): TitleDetails | null | undefined {
	const k = detailKey(a);
	return k ? (got[k] ?? null) : undefined;
}

/** wantDetails asks for an ask's details once. */
export function wantDetails(a: Pick<Ask, 'kind' | 'tmdb_id'>): void {
	const k = detailKey(a);
	if (!k || asked.has(k)) return;
	asked.add(k);
	const kind = a.kind === 'show' ? 'show' : 'movie';
	api
		.get<Record<string, unknown>>(`/metadata/${kind}/${encodeURIComponent(a.tmdb_id)}`)
		.then((raw) => (got[k] = fromMetadata(kind, a.tmdb_id, raw)))
		.catch(() => (got[k] = null));
}
