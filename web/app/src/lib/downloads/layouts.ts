// Shared sorting, grouping and wording for the Downloads layouts being
// compared (?view=1…5 on the Downloads page). Pure, so it can be tested.
import { nowPhases, type DownloadRow } from '../downloaders.ts';
import type { StatusKind } from '../ui/StatusPill.svelte';

/** The phases that are a problem: red edge, sorted first. */
export const problemPhases: ReadonlySet<string> = new Set(['stalled', 'failed', 'vanished', 'needs_a_decision', 'import_refused']);

export const isProblem = (r: Pick<DownloadRow, 'phase'>) => problemPhases.has(r.phase);
/** needsAttention: a problem, or paused in the Downloader (someone has to resume it). */
export const needsAttention = (r: Pick<DownloadRow, 'phase'>) => isProblem(r) || r.phase === 'paused';
/** moving: still has a progress bar worth drawing. */
export const moving = (r: Pick<DownloadRow, 'phase'>) => r.phase !== 'removed' && r.phase !== 'failed' && r.phase !== 'vanished';

// Problems first (the ones a person must act on), then what is moving, then
// paused, then what waits its turn, then what is finished.
const rank: Record<string, number> = {
	needs_a_decision: 0,
	import_refused: 1,
	failed: 2,
	stalled: 3,
	vanished: 4,
	paused: 5,
	importing: 6,
	waiting_to_import: 7,
	downloading: 8,
	waiting: 9,
	done: 10,
	removed: 11
};
export const phaseRank = (phase: string) => rank[phase] ?? 12;

/** byUrgency sorts problems first; downloads closest to done lead their group. */
export function byUrgency(a: DownloadRow, b: DownloadRow): number {
	return phaseRank(a.phase) - phaseRank(b.phase) || b.progress - a.progress || b.id - a.id;
}

/** queue is what the layouts show as "now": every row still moving, plus the
 *  finished ones that went wrong (failed, needs a decision), problems first. */
export function queue(rows: DownloadRow[]): DownloadRow[] {
	return rows.filter((r) => nowPhases.has(r.phase) || isProblem(r)).sort(byUrgency);
}

/** finished: done or removed, newest first. */
export function finished(rows: DownloadRow[]): DownloadRow[] {
	const when = (r: DownloadRow) => Date.parse(r.finished_at ?? r.updated_at) || 0;
	return rows.filter((r) => r.phase === 'done' || r.phase === 'removed').sort((a, b) => when(b) - when(a));
}

export type Lane = 'downloading' | 'waiting' | 'attention' | 'done';
/** laneOf places a row in one of the four lanes (null: removed, not shown). */
export function laneOf(r: Pick<DownloadRow, 'phase'>): Lane | null {
	if (needsAttention(r)) return 'attention';
	if (r.phase === 'downloading' || r.phase === 'importing' || r.phase === 'waiting_to_import') return 'downloading';
	if (r.phase === 'waiting') return 'waiting';
	if (r.phase === 'done') return 'done';
	return null;
}

/** lanes splits rows into the four lanes, each sorted. */
export function lanes(rows: DownloadRow[]): Record<Lane, DownloadRow[]> {
	const out: Record<Lane, DownloadRow[]> = { downloading: [], waiting: [], attention: [], done: [] };
	for (const r of rows) {
		const l = laneOf(r);
		if (l) out[l].push(r);
	}
	out.downloading.sort(byUrgency);
	out.waiting.sort(byUrgency);
	out.attention.sort(byUrgency);
	out.done = finished(out.done);
	return out;
}

/** doneToday: finished in the last 24 hours. */
export function doneToday(rows: DownloadRow[], now = Date.now()): DownloadRow[] {
	return rows.filter((r) => {
		const t = Date.parse(r.finished_at ?? r.updated_at);
		return !Number.isNaN(t) && now - t < 86_400_000;
	});
}

/** note is the row's message, or null when it only repeats the percentage
 *  ("18% done."), which the progress already says. */
export function note(r: Pick<DownloadRow, 'text'>): string | null {
	const t = r.text?.trim();
	if (!t) return null;
	if (/^\d+(?:[.,]\d+)?\s*%(?:\s*done)?\.?$/i.test(t)) return null;
	return t;
}

export interface TitleGroup {
	key: string;
	title_id?: number;
	name: string;
	kind?: DownloadRow['kind'];
	rows: DownloadRow[];
}

/** groupByTitle keeps the given order and puts every row of one show (or
 *  movie, or album) together under its first row's place. */
export function groupByTitle(rows: DownloadRow[], nameOf: (r: DownloadRow) => string): TitleGroup[] {
	const out: TitleGroup[] = [];
	const at = new Map<string, TitleGroup>();
	for (const r of rows) {
		const key = r.title_id ? `t${r.title_id}` : `n${r.id}`;
		const g = at.get(key);
		if (g) g.rows.push(r);
		else {
			const ng: TitleGroup = { key, title_id: r.title_id, name: nameOf(r), kind: r.kind, rows: [r] };
			at.set(key, ng);
			out.push(ng);
		}
	}
	for (const g of out) g.rows.sort((a, b) => (a.covers ?? '').localeCompare(b.covers ?? '', undefined, { numeric: true }) || byUrgency(a, b));
	return out;
}

/** pillKind picks the kit status colour for a row; the word stays the row's own. */
export function pillKind(r: Pick<DownloadRow, 'phase'>): StatusKind {
	if (r.phase === 'downloading' || r.phase === 'importing' || r.phase === 'waiting_to_import') return 'downloading';
	if (r.phase === 'done') return 'in_library';
	if (isProblem(r)) return 'failed';
	if (r.phase === 'paused' || r.phase === 'removed') return 'turned_down';
	return 'missing';
}

/** tone is the one colour word the layouts use for a row. */
export function tone(r: Pick<DownloadRow, 'phase'>): 'info' | 'ok' | 'fail' | 'idle' | 'wait' {
	const k = pillKind(r);
	return k === 'downloading' ? 'info' : k === 'in_library' ? 'ok' : k === 'failed' ? 'fail' : k === 'turned_down' ? 'idle' : 'wait';
}

export type SortKey = 'title' | 'state' | 'progress' | 'speed' | 'eta' | 'size' | 'downloader';

/** sortRows sorts the table; rows without a value (no speed, no time left) go last either way. */
export function sortRows(rows: DownloadRow[], key: SortKey, dir: 'asc' | 'desc', nameOf: (r: DownloadRow) => string): DownloadRow[] {
	const sign = dir === 'asc' ? 1 : -1;
	const val = (r: DownloadRow): number | string | null => {
		switch (key) {
			case 'title':
				return `${nameOf(r)} ${r.covers ?? ''}`.toLowerCase();
			case 'state':
				return phaseRank(r.phase);
			case 'progress':
				return r.progress;
			case 'speed':
				return r.active && r.speed ? r.speed : null;
			case 'eta':
				return r.active && r.eta ? r.eta : null;
			case 'size':
				return r.size || null;
			case 'downloader':
				return (r.downloader || '').toLowerCase() || null;
		}
	};
	return [...rows].sort((a, b) => {
		const x = val(a);
		const y = val(b);
		if (x === null && y === null) return byUrgency(a, b);
		if (x === null) return 1;
		if (y === null) return -1;
		const c = typeof x === 'string' ? x.localeCompare(String(y), undefined, { numeric: true }) : x - (y as number);
		return c * sign || byUrgency(a, b);
	});
}
