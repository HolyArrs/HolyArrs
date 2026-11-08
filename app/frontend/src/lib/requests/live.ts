// Where an approved ask is in the pipeline right now, from the local searches and downloads lists (no outside calls): one small entry per Library title.
import { notAlbumName } from './ask-parts.ts';
import type { LifeFacts } from './lifecycle.ts';
import type { AskStep } from '../core/types.ts';
import type { SearchView } from '../core/types.ts';
import type { DownloadRow } from '../core/types.ts';

export interface TitleLive {
	searching?: boolean;
	download?: { phase: 'downloading' | 'importing' | 'stuck' | 'failed'; progress: number; reason?: string; id?: number };
}

/** What a title page says about its parts (seasons or albums), as much as the window needs. */
export interface PartCounts {
	/** Season number or album mbid. */
	key: string | number;
	/** The whole part is in the Library. */
	complete: boolean;
	/** false: not counted in "n more available" (a single, remix, bootleg, or a season with nothing aired yet). */
	real?: boolean;
	/** Episodes aired or tracks the part holds, and how many of them are in the Library (for parts asked only in part). */
	size?: number;
	have?: number;
}

/** A part asked only in part: a season with single episodes, an album with kept songs. */
export interface PartPick {
	key: string | number;
	/** How many episodes or songs were kept. */
	count: number;
}

const DOWNLOADING = new Set(['waiting', 'downloading', 'stalled', 'paused']);
const IMPORTING = new Set(['importing', 'waiting_to_import']);
const FAILED = new Set(['failed', 'vanished']);

// Downloaded, but the import is waiting on a person (Downloads > Needs a decision).
const STUCK = new Set(['needs_a_decision', 'import_refused']);

/** Maps title id to what is running for it: running searches, and the freshest not-finished download. */
export function liveByTitle(downloads: (Pick<DownloadRow, 'title_id' | 'phase' | 'progress' | 'replaced'> & { text?: string; id?: number })[], running: Pick<SearchView, 'title_id' | 'state' | 'kind'>[]): Map<number, TitleLive> {
	const out = new Map<number, TitleLive>();
	const at = (id: number) => out.get(id) ?? out.set(id, {}).get(id)!;
	for (const s of running) {
		if (s.title_id && s.kind !== 'feed_check' && (s.state === 'running' || s.state === 'queued')) at(s.title_id).searching = true;
	}
	// Prefer a copy still moving (downloading over importing) to a failed one.
	const rank = { downloading: 0, importing: 1, stuck: 2, failed: 3 } as const;
	for (const d of downloads) {
		if (!d.title_id || d.replaced) continue;
		const phase = DOWNLOADING.has(d.phase) ? 'downloading' : IMPORTING.has(d.phase) ? 'importing' : STUCK.has(d.phase) ? 'stuck' : FAILED.has(d.phase) ? 'failed' : null;
		if (!phase) continue;
		const t = at(d.title_id);
		if (t.download && rank[phase] >= rank[t.download.phase]) continue;
		t.download = { phase, progress: d.progress, id: d.id };
		// A failed copy carries the server's reason ("no progress for 6 hours.").
		if (phase === 'failed' && d.text?.trim()) t.download.reason = d.text.trim();
	}
	return out;
}

/** Turns one title's entry into the lifecycle facts: the download's stage first, else searching. */
export function liveFacts(t: TitleLive | undefined): LifeFacts {
	if (t?.download && t.download.phase !== 'failed') return { phase: t.download.phase, progress: t.download.progress, downloadId: t.download.id };
	if (t?.searching) return { phase: 'searching' };
	if (t?.download) return t.download.reason ? { phase: 'failed', reason: t.download.reason, downloadId: t.download.id } : { phase: 'failed', downloadId: t.download.id };
	return {};
}

/** Reads the server's step for an approved ask: what the page says when it has no live search or download of its own for it. */
export function stepFacts(step: AskStep | undefined): LifeFacts {
	switch (step?.kind) {
		case 'searching':
			return { phase: 'searching' };
		case 'downloading':
			return { phase: 'downloading' };
		case 'importing':
			return { phase: 'importing' };
		case 'waiting_on_admin':
			return { phase: 'stuck' };
		case 'not_out':
			return { upcoming: '' };
		case 'waiting': {
			const f: LifeFacts = { phase: 'waiting' };
			if (step.text) f.reason = step.text;
			if (step.searched_at) f.searchedAt = step.searched_at;
			if (step.next_search) f.nextSearch = step.next_search;
			return f;
		}
	}
	return {};
}

// Title words that mark a release as not a real album (MusicBrainz sometimes leaves these untyped).
const NOT_ALBUM = /\b(demos?|bootlegs?|outtakes?|unreleased|rehearsals?|b-sides|leaked|snippets?|remix(es)?|remixed)\b/i;

/** Whether an album is a studio album: type album, no secondary types (live, compilation, remix), no bootleg words. */
export function realAlbum(a: { type?: string; secondary_types?: string[] | null; name?: string }): boolean {
	return (a.type ?? '').toLowerCase() === 'album' && !(a.secondary_types ?? []).length && !notAlbumName.test(a.name ?? '');
}

/** Counts the parts the ask covers that are fully in the Library. `asked` is the ask's whole seasons or album ids (empty, with no picks, means everything). `picks` are parts asked only in part: one is in once its kept episodes or songs are. */
export function partialFacts(unit: 'seasons' | 'albums', parts: PartCounts[], asked: (string | number)[], picks: PartPick[] = []): LifeFacts {
	if (!parts.length) return {};
	const pick = new Map(picks.filter((p) => !asked.includes(p.key)).map((p) => [p.key, p.count]));
	const want = asked.length || pick.size ? parts.filter((p) => asked.includes(p.key) || pick.has(p.key)) : parts;
	if (!want.length) return {};
	let skipped = 0;
	let have = 0;
	for (const p of want) {
		const n = pick.get(p.key);
		if (n === undefined || p.size === undefined) {
			if (p.complete) have++;
			continue;
		}
		skipped += Math.max(p.size - n, 0);
		if (p.size > 0 && (p.have ?? 0) >= Math.min(n, p.size)) have++;
	}
	const out: LifeFacts = { partial: { have, total: want.length, unit }, more: parts.filter((p) => p.real !== false && !want.includes(p)).length };
	// Only single episodes or songs asked (no whole season or album): count those, so one asked episode reads "Ready · 1 episode", not "1 season". What is on disk is counted per season or album (unpicked ones too).
	if (!asked.length && want.every((p) => pick.has(p.key) && p.size !== undefined)) {
		let total = 0;
		for (const p of want) total += Math.min(pick.get(p.key) ?? 0, p.size ?? 0);
		if (total > 0) out.partial = { have: 0, total, unit: unit === 'seasons' ? 'episodes' : 'songs' };
	}
	if (skipped > 0) out.skipped = skipped;
	return out;
}
