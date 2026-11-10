// The status tabs on the Downloads table: All, Waiting, In progress, Problems, Done, with sub-statuses under Waiting, In progress and Problems (All and Done have none). Pure, so it can be tested.
import { t } from '#lib/core/i18n/i18n.svelte.ts';
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
	queued: { key: 'queued', get label() { return t('downloads.tabs.sub.queued.label'); }, stages: ['queued'], get hint() { return t('downloads.tabs.sub.queued.hint'); }, get empty() { return t('downloads.tabs.sub.queued.empty'); } },
	paused: { key: 'paused', get label() { return t('downloads.tabs.sub.paused.label'); }, stages: ['paused'], get hint() { return t('downloads.tabs.sub.paused.hint'); }, get empty() { return t('downloads.tabs.sub.paused.empty'); } },
	downloading: { key: 'downloading', get label() { return t('downloads.tabs.sub.downloading.label'); }, stages: ['downloading', 'stalled'], get hint() { return t('downloads.tabs.sub.downloading.hint'); }, get empty() { return t('downloads.tabs.sub.downloading.empty'); } },
	checking: { key: 'checking', get label() { return t('downloads.tabs.sub.checking.label'); }, stages: ['checking'], get hint() { return t('downloads.tabs.sub.checking.hint'); }, get empty() { return t('downloads.tabs.sub.checking.empty'); } },
	importing: { key: 'importing', get label() { return t('downloads.tabs.sub.importing.label'); }, stages: ['importing'], get hint() { return t('downloads.tabs.sub.importing.hint'); }, get empty() { return t('downloads.tabs.sub.importing.empty'); } },
	failed: { key: 'failed', get label() { return t('downloads.tabs.sub.failed.label'); }, stages: ['failed'], get hint() { return t('downloads.tabs.sub.failed.hint'); }, get empty() { return t('downloads.tabs.sub.failed.empty'); } },
	decide: { key: 'decide', get label() { return t('downloads.tabs.sub.decide.label'); }, stages: ['decide'], get hint() { return t('downloads.tabs.sub.decide.hint'); }, get empty() { return t('downloads.tabs.sub.decide.empty'); } }
};

export const TABS: Tab[] = [
	{ key: 'active', get label() { return t('downloads.tabs.tab.active.label'); }, stages: ['queued', 'paused', 'downloading', 'stalled', 'checking', 'importing', 'failed', 'decide', 'done', 'seeding'], get hint() { return t('downloads.tabs.all.hint'); }, get empty() { return t('downloads.tabs.tab.active.empty'); }, subs: [] },
	{ key: 'waiting', get label() { return t('downloads.tabs.tab.waiting.label'); }, stages: ['queued', 'paused'], get hint() { return t('downloads.tabs.tab.waiting.hint'); }, get empty() { return t('downloads.tabs.tab.waiting.empty'); }, subs: [SUBS.queued, SUBS.paused] },
	{
		key: 'progress',
		get label() { return t('downloads.tabs.tab.progress.label'); },
		stages: ['downloading', 'stalled', 'checking', 'importing'],
		get hint() { return t('downloads.tabs.tab.progress.hint'); },
		get empty() { return t('downloads.tabs.tab.progress.empty'); },
		subs: [SUBS.downloading, SUBS.checking, SUBS.importing]
	},
	{ key: 'problems', get label() { return t('downloads.tabs.tab.problems.label'); }, stages: ['failed', 'decide'], get hint() { return t('downloads.tabs.tab.problems.hint'); }, get empty() { return t('downloads.tabs.tab.problems.empty'); }, subs: [SUBS.failed, SUBS.decide] },
	{
		key: 'done',
		get label() { return t('downloads.tabs.tab.done.label'); },
		stages: ['done', 'seeding'],
		get hint() { return t('downloads.tabs.tab.done.hint'); },
		get empty() { return t('downloads.tabs.tab.done.empty'); },
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
	if (r.phase === 'failed' || r.phase === 'vanished') return t('downloads.status.failed');
	switch (stageOf(r)) {
		case 'checking':
			return r.client_hint === 'moving' ? t('downloads.status.moving') : t('downloads.status.checking');
		case 'queued':
			return t('downloads.status.queued');
		case 'paused':
			return t('downloads.status.paused');
		case 'downloading':
			return t('downloads.status.downloading');
		case 'stalled':
			return t('downloads.status.stalled');
		case 'importing':
			return t('downloads.status.importing');
		case 'seeding':
			return t('downloads.status.seeding');
		case 'decide':
			return t('downloads.status.review');
		case 'failed':
			return t('downloads.status.failed');
	}
	return t('downloads.status.done');
}

/** A quiet sentence for the tooltip after the status word, or ''. */
export function statusHint(r: RowBits): string {
	switch (r.client_hint) {
		case 'idle_seed':
			return t('downloads.status.idleSeed');
		case 'moving':
			return t('downloads.status.toFolder');
	}
	return r.phase === 'stalled' ? t('downloads.status.noData') : '';
}

/** The short second line under the status word when nothing is wrong and the row is not in a queue: the step an import is at, why a download failed, or a seeding torrent's ratio. Two words at most. */
export function stepLine(r: Pick<RowBits, 'client_hint'>, detail: string): string {
	switch (r.client_hint) {
		case 'verifying':
			return t('downloads.step.verifying');
		case 'repairing':
			return t('downloads.step.repairing');
		case 'unpacking':
			return t('downloads.step.unpacking');
		case 'finishing':
			return t('downloads.step.finishing');
		case 'script':
			return t('downloads.step.script');
		case 'missing':
			return t('downloads.step.missing');
		case 'errored':
			return t('downloads.step.errored');
	}
	if (detail.startsWith(t('downloads.detail.importFailed', { why: '' }).trimEnd())) return t('downloads.step.importFailed');
	if (detail === t('downloads.detail.removed')) return t('downloads.step.removedOutside');
	if (detail === t('downloads.detail.noMatch')) return t('downloads.step.noMatch');
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
	active: { get text() { return t('downloads.empty.active'); }, glyph: 'down' },
	waiting: { get text() { return t('downloads.empty.waiting'); }, glyph: 'clock' },
	progress: { get text() { return t('downloads.empty.progress'); }, glyph: 'down' },
	problems: { get text() { return t('downloads.empty.problems'); }, glyph: 'check' },
	done: { get text() { return t('downloads.empty.done'); }, glyph: 'done' }
};

const KIND_NOUN: Record<string, string> = {
	get movie() { return t('downloads.noun.movie'); },
	get show() { return t('downloads.noun.show'); },
	get artist() { return t('downloads.noun.music'); },
	usenet: 'Usenet',
	torrent: 'torrent'
};

/** The empty-state card for the list as filtered: a search with no hits, a kind filter with no rows, or the tab (or sub-tab) itself. */
export function emptyOf(tab: string, sub = '', term = '', kind = '', tabs: Tab[] = TABS, source = ''): Empty {
	const q = term.trim();
	if (q) return { title: t('downloads.empty.noMatches', { q }), text: '', glyph: 'search', action: { label: t('downloads.list.clearSearch'), act: 'clear' } };
	if (kind && KIND_NOUN[kind]) {
		return { title: t('downloads.empty.noKind', { kind: KIND_NOUN[kind] }), text: '', glyph: 'filter', action: { label: t('downloads.empty.showAll'), act: 'all' } };
	}
	if (source && KIND_NOUN[source]) {
		return { title: t('downloads.empty.noKind', { kind: KIND_NOUN[source] }), text: '', glyph: 'filter', action: { label: t('downloads.empty.showAll'), act: 'all' } };
	}
	const tb = tabs.find((x) => x.key === tab) ?? tabs[0];
	const s = tb.subs.find((x) => x.key === sub);
	const base = EMPTY[tb.key] ?? EMPTY.active;
	if (s) return { title: s.empty.replace(/\.$/, ''), text: s.hint, glyph: tb.key === 'problems' ? 'check' : base.glyph };
	return { title: tb.empty, ...base };
}
