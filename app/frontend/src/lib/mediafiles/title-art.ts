// Fetches the first artwork URL per title, cached briefly and shared between callers.
import { api } from '../core/api.ts';

type Brief = { id: number; artwork?: string[] };

let kept: { at: number; map: Map<number, string> } | null = null;
let inflight: Promise<Map<number, string>> | null = null;

// Reuses the last map for 60s and joins a request already in flight; on failure falls back to the last map.
export function titleArt(): Promise<Map<number, string>> {
	if (kept && Date.now() - kept.at < 60_000) return Promise.resolve(kept.map);
	if (inflight) return inflight;
	inflight = api
		.get<{ titles: Brief[] } | Brief[]>('/titles?brief=1')
		.then((r) => {
			const list = Array.isArray(r) ? r : (r.titles ?? []);
			const map = new Map<number, string>();
			for (const t of list) if (t.artwork?.[0]) map.set(t.id, t.artwork[0]);
			kept = { at: Date.now(), map };
			return map;
		})
		.catch(() => kept?.map ?? new Map<number, string>())
		.finally(() => (inflight = null));
	return inflight;
}
