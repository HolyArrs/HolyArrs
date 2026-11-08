// The status tabs on the Downloads table: All, Waiting, In progress, Problems, Done, with sub-statuses under Waiting, In progress and Problems (All and Done have none). Pure, so it can be tested.
import type { DownloadRow } from '../core/types.ts';

export type Stage = 'checking' | 'queued' | 'paused' | 'downloading' | 'stalled' | 'importing' | 'done' | 'seeding' | 'failed' | 'decide';

type RowBits = Pick<DownloadRow, 'phase' | 'state' | 'progress'> & { replaced?: boolean; client_hint?: DownloadRow['client_hint'] };

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

export interface Counted {
	key: string;
	label: string;
	count: number;
	hint: string;
}

export type EmptyGlyph = 'down' | 'clock' | 'check' | 'done' | 'search' | 'filter';
export type EmptyAction = { label: string; act: 'clear' | 'all' };

export interface Empty {
	title: string;
	text: string;
	glyph: EmptyGlyph;
	action?: EmptyAction;
}

/** Places one row on its fine-grained stage. */
export function stageOf(r: RowBits): Stage {
	// A torrent checking its data or moving to a new folder is busy, not waiting.
	if (r.client_hint === 'checking' || r.client_hint === 'moving') return 'checking';
	switch (r.phase) {
		case 'waiting':
			return 'queued';
		case 'paused':
			return 'paused';
		case 'downloading':
			// A Usenet job at full progress is being verified, repaired or unpacked by the Downloader. The bytes are in, so it is importing.
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
			// Turned down and replaced (HolyArrs searches again or has another copy) counts as finished, not a problem.
			return r.replaced ? 'done' : 'failed';
		case 'import_refused':
			return 'failed';
		case 'done':
			return r.state === 'seeding' ? 'seeding' : 'done';
	}
	// Removed (cleaned up) and anything new count as finished.
	return 'done';
}

/** The default tab: every row, finished and seeding ones included. */
export const ALL = 'active';

export const ALL_HINT = 'Everything: waiting, on its way, needing you, or finished.';

const SUBS: Record<string, Sub> = {
	queued: { key: 'queued', label: 'Queued', stages: ['queued'], hint: 'Starts when the downloader has room.', empty: 'Nothing is queued.' },
	paused: { key: 'paused', label: 'Paused', stages: ['paused'], hint: 'Paused in the Downloader.', empty: 'Nothing is paused.' },
	downloading: { key: 'downloading', label: 'Downloading', stages: ['downloading', 'stalled'], hint: 'Taking in data, or stalled while it keeps trying.', empty: 'Nothing is downloading.' },
	checking: { key: 'checking', label: 'Checking', stages: ['checking'], hint: 'Checking its data on disk, or moving to a new folder.', empty: 'Nothing is being checked or moved.' },
	importing: { key: 'importing', label: 'Importing', stages: ['importing'], hint: 'Finished downloading; being checked and moved into your Library.', empty: 'Nothing is importing.' },
	failed: { key: 'failed', label: 'Failed', stages: ['failed'], hint: 'Failed, removed outside HolyArrs, or the files did not match.', empty: 'Nothing failed.' },
	decide: { key: 'decide', label: 'Review', stages: ['decide'], hint: 'HolyArrs could not finish it on its own and needs you to choose.', empty: 'Nothing to review.' }
};

export const TABS: Tab[] = [
	{ key: 'active', label: 'All', stages: ['queued', 'paused', 'downloading', 'stalled', 'checking', 'importing', 'failed', 'decide', 'done', 'seeding'], hint: ALL_HINT, empty: 'No downloads yet', subs: [] },
	{ key: 'waiting', label: 'Waiting', stages: ['queued', 'paused'], hint: "In your Downloader's line, or paused by you.", empty: 'Nothing waiting', subs: [SUBS.queued, SUBS.paused] },
	{
		key: 'progress',
		label: 'In progress',
		stages: ['downloading', 'stalled', 'checking', 'importing'],
		hint: 'On its way: downloading now, or being checked and filed into your Library.',
		empty: 'Nothing downloading',
		subs: [SUBS.downloading, SUBS.checking, SUBS.importing]
	},
	{ key: 'problems', label: 'Problems', stages: ['failed', 'decide'], hint: 'Stuck, failed, or waiting for you to decide.', empty: 'All clear', subs: [SUBS.failed, SUBS.decide] },
	{
		key: 'done',
		label: 'Done',
		stages: ['done', 'seeding'],
		hint: 'Finished: in your Library, or still being shared.',
		empty: 'Nothing finished yet',
		subs: []
	}
];

/** The tabs as shown. They are the same for every Downloader (Seeding is only a row word), so the arguments are kept for the callers. */
export function tabsFor(_rows?: RowBits[], _torrent?: boolean): Tab[] {
	return TABS;
}

/** Filters rows to one tab (and one sub-tab); unknown keys show all. */
export function pick<T extends RowBits>(rows: T[], tabs: Tab[], show: string, sub = ''): T[] {
	const t = tabs.find((x) => x.key === show);
	if (!t) return rows;
	const st = t.subs.find((x) => x.key === sub);
	const stages = new Set<Stage>(st ? st.stages : t.stages);
	return rows.filter((r) => stages.has(stageOf(r)));
}

/** Every tab with its row count. */
export function counts(rows: RowBits[], tabs: Tab[]): Counted[] {
	const stages = rows.map(stageOf);
	const out: Counted[] = [];
	for (const t of tabs) {
		const k: Counted = { key: t.key, label: t.label, count: stages.filter((s) => t.stages.includes(s)).length, hint: t.hint };
		out.push(k);
	}
	return out;
}

/** A tab's sub-tabs with their counts. */
export function subCounts(rows: RowBits[], tab: Tab | undefined): Counted[] {
	if (!tab) return [];
	const stages = rows.map(stageOf);
	return tab.subs.map((s) => ({ key: s.key, label: s.label, count: stages.filter((x) => s.stages.includes(x)).length, hint: s.hint }));
}

/** The row's word in the status column; it follows the sub-tab words. A download removed outside HolyArrs, a failed copy already replaced, or a torrent the Downloader marks errored or missing its files reads accordingly. */
export function statusWord(r: RowBits): string {
	if (r.phase === 'failed' || r.phase === 'vanished') return 'Failed';
	switch (stageOf(r)) {
		case 'checking':
			return r.client_hint === 'moving' ? 'Moving' : 'Checking';
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

/** A quiet sentence for the tooltip after the status word, or ''. */
export function statusHint(r: RowBits): string {
	switch (r.client_hint) {
		case 'idle_seed':
			return 'Nobody is asking for it right now';
		case 'moving':
			return 'To a new folder';
	}
	return r.phase === 'stalled' ? 'No data coming in; still trying' : '';
}

/** The short second line under the status word when nothing is wrong and the row is not in a queue: the step an import is at, why a download failed, or a seeding torrent's ratio. Two words at most. */
export function stepLine(r: Pick<RowBits, 'client_hint'>, detail: string): string {
	switch (r.client_hint) {
		case 'verifying':
			return 'Verifying';
		case 'repairing':
			return 'Repairing';
		case 'unpacking':
			return 'Unpacking';
		case 'finishing':
			return 'Finishing';
		case 'script':
			return 'Running script';
		case 'missing':
			return 'Files missing';
		case 'errored':
			return 'Downloader error';
	}
	if (detail.startsWith('import failed')) return 'Import failed';
	if (detail === 'removed outside HolyArrs') return 'Removed outside';
	if (detail === 'the files did not match') return 'Files did not match';
	return detail ? detail.charAt(0).toUpperCase() + detail.slice(1) : '';
}

/** Trims a Downloader's own failure line for the cell: its status code in brackets goes, and so does the Downloader's name, which the Downloader column already says. The tooltip keeps the full line. */
export function shortReason(s: string): string {
	return s
		.replace(/\s*\([A-Z_]+\/[A-Z_]+\)\s*$/, '')
		.replace(/ in (NZBGet|SABnzbd)\b/, '')
		.replace(/ were missing$/, ' missing')
		.trim();
}

/** Values older links used, mapped to the nearest group now. */
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

/** Reads ?show=: a tab key, or tab.sub; an old value maps to the nearest group, anything else is All. */
export function showOf(v: string | null | undefined, tabs: Tab[] = TABS): { show: string; sub: string } {
	const raw = v ?? '';
	const [a = '', b = ''] = (LEGACY[raw] ?? raw).split('.');
	const t = tabs.find((x) => x.key === a);
	if (!t) return { show: ALL, sub: '' };
	return { show: t.key, sub: t.subs.some((x) => x.key === b) ? b : '' };
}

const EMPTY: Record<string, Omit<Empty, 'title'>> = {
	active: { text: 'Anything HolyArrs or your Downloader is fetching shows up here.', glyph: 'down' },
	waiting: { text: 'Downloads queued or paused show up here.', glyph: 'clock' },
	progress: { text: 'Downloads in progress show up here with their speed.', glyph: 'down' },
	problems: { text: 'Failed downloads and ones that need a decision show up here.', glyph: 'check' },
	done: { text: 'Finished and seeding downloads show up here.', glyph: 'done' }
};

const KIND_NOUN: Record<string, string> = { movie: 'movie', show: 'TV show', artist: 'music', usenet: 'Usenet', torrent: 'torrent' };

/** The empty-state card for the list as filtered: a search with no hits, a kind filter with no rows, or the tab (or sub-tab) itself. */
export function emptyOf(tab: string, sub = '', term = '', kind = '', tabs: Tab[] = TABS, source = ''): Empty {
	const q = term.trim();
	if (q) return { title: `No matches for “${q}”`, text: '', glyph: 'search', action: { label: 'Clear search', act: 'clear' } };
	if (kind && KIND_NOUN[kind]) {
		return { title: `No ${KIND_NOUN[kind]} downloads`, text: '', glyph: 'filter', action: { label: 'Show all', act: 'all' } };
	}
	if (source && KIND_NOUN[source]) {
		return { title: `No ${KIND_NOUN[source]} downloads`, text: '', glyph: 'filter', action: { label: 'Show all', act: 'all' } };
	}
	const t = tabs.find((x) => x.key === tab) ?? tabs[0];
	const s = t.subs.find((x) => x.key === sub);
	const base = EMPTY[t.key] ?? EMPTY.active;
	if (s) return { title: s.empty.replace(/\.$/, ''), text: s.hint, glyph: t.key === 'problems' ? 'check' : base.glyph };
	return { title: t.empty, ...base };
}
