// The status tabs on the Downloads table: All · Waiting · In progress ·
// Problems · Done, with sub-statuses under Waiting, In progress and Problems
// (All and Done have none). Pure, so it can be tested.
//
// The words mean the same thing for a torrent and a Usenet download:
// Downloading is bytes coming in (a stalled download is still trying);
// Importing is everything after the bytes arrive (Usenet verify, repair and
// unpack, waiting to import, and putting the files in the Library); Seeding
// is a finished torrent the Downloader still shares. ("Active" is not used:
// in torrent Downloaders it also counts finished torrents still seeding.)
// Stalled and Seeding are one-word row statuses only: a stalled download
// counts under In progress > Downloading, a seeding one under Done, and a
// failed one only under Problems.
import type { DownloadRow } from '../downloaders.ts';

export type Stage = 'queued' | 'paused' | 'downloading' | 'stalled' | 'importing' | 'done' | 'seeding' | 'failed' | 'decide';

type RowBits = Pick<DownloadRow, 'phase' | 'state' | 'progress'> & { replaced?: boolean };

/** stageOf places one row on its fine-grained stage. */
export function stageOf(r: RowBits): Stage {
	switch (r.phase) {
		case 'waiting':
			return 'queued';
		case 'paused':
			return 'paused';
		case 'downloading':
			// A Usenet job at full progress is being verified, repaired or
			// unpacked by the Downloader: the bytes are in, so it is importing.
			return r.progress >= 1 ? 'importing' : 'downloading';
		case 'stalled':
			return 'stalled';
		case 'waiting_to_import':
		case 'importing':
			return 'importing';
		case 'needs_a_decision':
			return 'decide';
		case 'failed':
		case 'vanished':
			// Turned down and replaced (HolyArrs searches again or has another
			// copy): finished as far as the table goes, not a problem.
			return r.replaced ? 'done' : 'failed';
		case 'import_refused':
			return 'failed';
		case 'done':
			return r.state === 'seeding' ? 'seeding' : 'done';
	}
	// removed (cleaned up) and anything new: finished as far as the table goes.
	return 'done';
}

export type TabKey = 'active' | 'waiting' | 'progress' | 'done' | 'problems';

export interface Sub {
	key: string;
	label: string;
	stages: Stage[];
	/** One plain sentence: the tab's tooltip and its empty-state line. */
	hint: string;
	/** The empty-state title. */
	empty: string;
}

export interface Tab {
	key: TabKey;
	label: string;
	stages: Stage[];
	hint: string;
	empty: string;
	/** The small toggle tabs under this one when it is picked. */
	subs: Sub[];
}

/** ALL is the default tab: everything not yet finished (waiting, in progress and problems). */
export const ALL = 'active';
export const ALL_HINT = 'Everything not finished yet: waiting, on its way, or needing you.';

const SUBS: Record<string, Sub> = {
	queued: { key: 'queued', label: 'Queued', stages: ['queued'], hint: 'Not started yet; the Downloader starts it when there is room.', empty: 'Nothing is queued.' },
	paused: { key: 'paused', label: 'Paused', stages: ['paused'], hint: 'Paused in the Downloader.', empty: 'Nothing is paused.' },
	downloading: { key: 'downloading', label: 'Downloading', stages: ['downloading', 'stalled'], hint: 'Taking in data, or stalled while it keeps trying.', empty: 'Nothing is downloading.' },
	importing: { key: 'importing', label: 'Importing', stages: ['importing'], hint: 'Finished downloading; being checked and moved into your Library.', empty: 'Nothing is importing.' },
	failed: { key: 'failed', label: 'Failed', stages: ['failed'], hint: 'Failed, removed outside HolyArrs, or the files did not match.', empty: 'Nothing failed.' },
	decide: { key: 'decide', label: 'Review', stages: ['decide'], hint: 'HolyArrs could not finish it on its own and needs you to choose.', empty: 'Nothing to review.' }
};

export const TABS: Tab[] = [
	{ key: 'active', label: 'All', stages: ['queued', 'paused', 'downloading', 'stalled', 'importing', 'failed', 'decide'], hint: ALL_HINT, empty: 'Nothing is downloading or waiting.', subs: [] },
	{ key: 'waiting', label: 'Waiting', stages: ['queued', 'paused'], hint: "In your Downloader's line, or paused by you.", empty: 'Nothing waiting.', subs: [SUBS.queued, SUBS.paused] },
	{
		key: 'progress',
		label: 'In progress',
		stages: ['downloading', 'stalled', 'importing'],
		hint: 'On its way: downloading now, or being checked and filed into your Library.',
		empty: 'Nothing on its way right now.',
		subs: [SUBS.downloading, SUBS.importing]
	},
	{ key: 'problems', label: 'Problems', stages: ['failed', 'decide'], hint: 'Stuck, failed, or waiting for you to decide.', empty: 'No problems. Everything is moving.', subs: [SUBS.failed, SUBS.decide] },
	{
		key: 'done',
		label: 'Done',
		stages: ['done', 'seeding'],
		hint: 'Finished: in your Library, or still being shared.',
		empty: 'Nothing finished yet.',
		subs: []
	}
];

/** tabsFor is the tabs as shown. They are the same for every Downloader (Seeding is only a row word), so the arguments are kept for the callers. */
export function tabsFor(_rows?: RowBits[], _torrent?: boolean): Tab[] {
	return TABS;
}

/** pick filters rows to one tab (and one sub-tab); unknown keys show all. */
export function pick<T extends RowBits>(rows: T[], tabs: Tab[], show: string, sub = ''): T[] {
	const t = tabs.find((x) => x.key === show);
	if (!t) return rows;
	const st = t.subs.find((x) => x.key === sub);
	const stages = new Set<Stage>(st ? st.stages : t.stages);
	return rows.filter((r) => stages.has(stageOf(r)));
}

export interface Counted {
	key: string;
	label: string;
	count: number;
	hint: string;
}

/** counts gives every tab with its row count. */
export function counts(rows: RowBits[], tabs: Tab[]): Counted[] {
	const stages = rows.map(stageOf);
	const out: Counted[] = [];
	for (const t of tabs) {
		const k: Counted = { key: t.key, label: t.label, count: stages.filter((s) => t.stages.includes(s)).length, hint: t.hint };
		out.push(k);
	}
	return out;
}

/** subCounts gives a tab's sub-tabs with their counts. */
export function subCounts(rows: RowBits[], tab: Tab | undefined): Counted[] {
	if (!tab) return [];
	const stages = rows.map(stageOf);
	return tab.subs.map((s) => ({ key: s.key, label: s.label, count: stages.filter((x) => s.stages.includes(x)).length, hint: s.hint }));
}

/** statusWord is the row's word in the status column; it follows the sub-tab
 *  words. A download removed outside HolyArrs, or a failed copy already
 *  replaced, still reads Failed; one cleaned up from HolyArrs reads Done. */
export function statusWord(r: RowBits): string {
	if (r.phase === 'failed' || r.phase === 'vanished') return 'Failed';
	switch (stageOf(r)) {
		case 'queued':
			return 'Queued';
		case 'paused':
			return 'Paused';
		case 'downloading':
			return 'Downloading';
		case 'stalled':
			return 'Stalled';
		case 'importing':
			return 'Importing';
		case 'seeding':
			return 'Seeding';
		case 'decide':
			return 'Review';
		case 'failed':
			return 'Failed';
	}
	return 'Done';
}

/** The values older links used, mapped to the nearest group now. */
const LEGACY: Record<string, string> = {
	all: 'active',
	stalled: 'progress.downloading',
	'progress.stalled': 'progress.downloading',
	seeding: 'done',
	'done.seeding': 'done',
	'done.done': 'done',
	failed: 'problems.failed',
	'done.failed': 'problems.failed',
	decide: 'problems.decide'
};

/** showOf reads ?show= : a tab key, or tab.sub; an old value maps to the nearest group, anything else is All. */
export function showOf(v: string | null | undefined, tabs: Tab[] = TABS): { show: string; sub: string } {
	const raw = v ?? '';
	const [a = '', b = ''] = (LEGACY[raw] ?? raw).split('.');
	const t = tabs.find((x) => x.key === a);
	if (!t) return { show: ALL, sub: '' };
	return { show: t.key, sub: t.subs.some((x) => x.key === b) ? b : '' };
}
