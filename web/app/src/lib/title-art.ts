// The poster of every Library title by id, for lists that only know a title
// id (the Library tools). One brief read of the Library, kept for a minute.

import { api } from './api.ts';

type Brief = { id: number; artwork?: string[] };
let kept: { at: number; map: Map<number, string> } | null = null;
let inflight: Promise<Map<number, string>> | null = null;

/** titleArt answers a map of title id to its first picture (missing ids are simply absent). */
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
