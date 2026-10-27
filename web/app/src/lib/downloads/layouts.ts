// Sorting and colour words for the Downloads table. Pure, so it can be tested.
import type { DownloadRow } from '../downloaders.ts';
import type { StatusKind } from '../ui/StatusPill.svelte';

/** The phases that are a problem: red edge, sorted first. */
export const problemPhases: ReadonlySet<string> = new Set(['stalled', 'failed', 'vanished', 'needs_a_decision', 'import_refused']);

export const isProblem = (r: Pick<DownloadRow, 'phase'>) => problemPhases.has(r.phase);

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
