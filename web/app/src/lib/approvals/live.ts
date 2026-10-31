// Where an approved ask is in the pipeline right now, from the local searches
// and downloads lists (no outside calls): one small entry per Library title.
import type { SearchView } from '../types.ts';
import type { DownloadRow } from '../downloaders.ts';
import type { LifeFacts } from './lifecycle.ts';

export interface TitleLive {
	searching?: boolean;
	download?: { phase: 'downloading' | 'importing' | 'failed'; progress: number };
}

const DOWNLOADING = new Set(['waiting', 'downloading', 'stalled', 'paused']);
const IMPORTING = new Set(['importing', 'waiting_to_import']);
const FAILED = new Set(['failed', 'vanished']);

/** liveByTitle maps title id to what is running for it: running searches, and the freshest not-finished download. */
export function liveByTitle(downloads: Pick<DownloadRow, 'title_id' | 'phase' | 'progress' | 'replaced'>[], running: Pick<SearchView, 'title_id' | 'state' | 'kind'>[]): Map<number, TitleLive> {
	const out = new Map<number, TitleLive>();
	const at = (id: number) => out.get(id) ?? out.set(id, {}).get(id)!;
	for (const s of running) {
		if (s.title_id && s.kind !== 'feed_check' && (s.state === 'running' || s.state === 'queued')) at(s.title_id).searching = true;
	}
	// Prefer a copy still moving (downloading over importing) to a failed one.
	const rank = { downloading: 0, importing: 1, failed: 2 } as const;
	for (const d of downloads) {
		if (!d.title_id || d.replaced) continue;
		const phase = DOWNLOADING.has(d.phase) ? 'downloading' : IMPORTING.has(d.phase) ? 'importing' : FAILED.has(d.phase) ? 'failed' : null;
		if (!phase) continue;
		const t = at(d.title_id);
		if (!t.download || rank[phase] < rank[t.download.phase]) t.download = { phase, progress: d.progress };
	}
	return out;
}

/** liveFacts turns one title's entry into the lifecycle facts: the download's stage first, else searching. */
export function liveFacts(t: TitleLive | undefined): LifeFacts {
	if (t?.download && t.download.phase !== 'failed') return { phase: t.download.phase, progress: t.download.progress };
	if (t?.searching) return { phase: 'searching' };
	if (t?.download) return { phase: 'failed' };
	return {};
}

/** What a title page says about its parts (seasons or albums), as much as the window needs. */
export interface PartCounts {
	/** Season number or album mbid. */
	key: string | number;
	/** Whole part is in the Library. */
	complete: boolean;
}

/**
 * partialFacts counts the parts the ask covers that are fully in the Library.
 * `asked` is the ask's seasons or album ids (empty: everything). Parts the
 * ask did not cover are counted as `more`.
 */
export function partialFacts(unit: 'seasons' | 'albums', parts: PartCounts[], asked: (string | number)[]): LifeFacts {
	if (!parts.length) return {};
	const want = asked.length ? parts.filter((p) => asked.includes(p.key)) : parts;
	if (!want.length) return {};
	return { partial: { have: want.filter((p) => p.complete).length, total: want.length, unit }, more: parts.length - want.length };
}
