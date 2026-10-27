// The status tabs on the Downloads table: All · Waiting · In progress · Done ·
// Problems, with small toggle sub-tabs under the picked one. Pure, so it can
// be tested.
//
// The words mean the same thing for a torrent and a Usenet download:
// Downloading is bytes coming in (a stalled download is still trying);
// Importing is everything after the bytes arrive (Usenet verify, repair and
// unpack, waiting to import, and putting the files in the Library); Seeding
// is a finished torrent the Downloader still shares. ("Active" is not used:
// in torrent Downloaders it also counts finished torrents still seeding.)
import type { DownloadRow } from '../downloaders.ts';

export type Stage = 'queued' | 'paused' | 'downloading' | 'stalled' | 'importing' | 'done' | 'seeding' | 'failed' | 'decide';

type RowBits = Pick<DownloadRow, 'phase' | 'state' | 'progress'>;

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
		case 'import_refused':
			return 'failed';
		case 'done':
			return r.state === 'seeding' ? 'seeding' : 'done';
	}
	// removed (cleaned up) and anything new: finished as far as the table goes.
	return 'done';
}

export type TabKey = 'waiting' | 'progress' | 'done' | 'problems';

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
	/** Show the seeding sub-count on this tab ("Done 12 · 4 seeding"). */
	seedCount?: boolean;
	/** The small toggle tabs under this one when it is picked. */
	subs: Sub[];
}

export const ALL = 'all';
export const ALL_HINT = 'Every download, whatever its state.';

const SUBS: Record<string, Sub> = {
	queued: { key: 'queued', label: 'Queued', stages: ['queued'], hint: 'Not started yet; the Downloader starts it when there is room.', empty: 'Nothing is queued.' },
	paused: { key: 'paused', label: 'Paused', stages: ['paused'], hint: 'Paused in the Downloader.', empty: 'Nothing is paused.' },
	downloading: { key: 'downloading', label: 'Downloading', stages: ['downloading', 'stalled'], hint: 'Taking in data, or stalled while it keeps trying.', empty: 'Nothing is downloading.' },
	importing: { key: 'importing', label: 'Importing', stages: ['importing'], hint: 'Finished downloading; being checked and moved into your Library.', empty: 'Nothing is importing.' },
	done: { key: 'done', label: 'Done', stages: ['done'], hint: 'Finished and in your Library.', empty: 'Nothing finished yet.' },
	seeding: { key: 'seeding', label: 'Seeding', stages: ['seeding'], hint: 'In your Library; the torrent is still being shared.', empty: 'Nothing is seeding.' },
	failed: { key: 'failed', label: 'Failed', stages: ['failed'], hint: 'Failed, removed outside HolyArrs, or the files did not match.', empty: 'Nothing failed.' },
	decide: { key: 'decide', label: 'Needs a decision', stages: ['decide'], hint: 'HolyArrs could not finish it on its own and needs you to choose.', empty: 'Nothing needs a decision.' }
};

export const TABS: Tab[] = [
	{ key: 'waiting', label: 'Waiting', stages: ['queued', 'paused'], hint: "In your Downloader's line, or paused by you.", empty: 'Nothing waiting.', subs: [SUBS.queued, SUBS.paused] },
	{
		key: 'progress',
		label: 'In progress',
		stages: ['downloading', 'stalled', 'importing'],
		hint: 'On its way: downloading now, or being checked and filed into your Library.',
		empty: 'Nothing on its way right now.',
		subs: [SUBS.downloading, SUBS.importing]
	},
	{
		key: 'done',
		label: 'Done',
		stages: ['done', 'seeding'],
		seedCount: true,
		hint: 'In your Library. Torrents still sharing with others count here.',
		empty: 'Nothing finished yet.',
		subs: [SUBS.done, SUBS.seeding]
	},
	{ key: 'problems', label: 'Problems', stages: ['failed', 'decide'], hint: 'Stuck, failed, or waiting for you to decide.', empty: 'No problems. Everything is moving.', subs: [SUBS.failed, SUBS.decide] }
];

/** tabsFor is the tabs as shown: the Seeding sub-tab only with a torrent
 *  Downloader or a seeding row; without it Done takes any seeding rows. */
export function tabsFor(rows: RowBits[], torrent: boolean): Tab[] {
	if (torrent || rows.some((r) => stageOf(r) === 'seeding')) return TABS;
	return TABS.map((t) => (t.key === 'done' ? { ...t, subs: [{ ...SUBS.done, stages: ['done', 'seeding'] }] } : t));
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
	/** Rows of this tab that are seeding, when the tab shows that sub-count. */
	seeding?: number;
}

/** counts gives All plus every tab with its row count. */
export function counts(rows: RowBits[], tabs: Tab[]): Counted[] {
	const stages = rows.map(stageOf);
	const out: Counted[] = [{ key: ALL, label: 'All', count: rows.length, hint: ALL_HINT }];
	for (const t of tabs) {
		const k: Counted = { key: t.key, label: t.label, count: stages.filter((s) => t.stages.includes(s)).length, hint: t.hint };
		if (t.seedCount) k.seeding = stages.filter((s) => s === 'seeding').length;
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
 *  words. A cleaned-up download reads Removed. */
export function statusWord(r: RowBits): string {
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
			return 'Needs a decision';
		case 'failed':
			return 'Failed';
	}
	return r.phase === 'removed' ? 'Removed' : 'Done';
}

/** showOf reads ?show= : a tab key, or tab.sub; anything else is All. */
export function showOf(v: string | null | undefined, tabs: Tab[] = TABS): { show: string; sub: string } {
	const [a = '', b = ''] = (v ?? '').split('.');
	const t = tabs.find((x) => x.key === a);
	if (!t) return { show: ALL, sub: '' };
	return { show: t.key, sub: t.subs.some((s) => s.key === b) ? b : '' };
}
