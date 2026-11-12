// History tabs shared with the Downloads table. Five big tabs answer what a person opens History for: what came in, what went wrong, what changed, what ran in the background.
import { t as say } from '#lib/core/i18n/i18n.svelte.ts';
import { details, groupDownloads, type HistoryRow } from './history.ts';
import { dayKey, rowTier, type Tier } from './history-kinds.ts';
import type { EventRecord } from '../core/types.ts';

export interface HistorySubTab {
	key: string;
	label: string;
	hint: string;
	/** Event kinds; a kind ending in * is a prefix. */
	kinds: string[];
}

export interface HistoryTab {
	key: string;
	label: string;
	hint: string;
	/** What the tab says when it is empty. */
	empty: string;
	subs: HistorySubTab[];
}

/** One day of rows: key, label and a count of what mattered. */
export interface DayGroup {
	key: string;
	rows: HistoryRow[];
	/** Rows per big tab, for the day's summary line. */
	tally: Record<string, number>;
}

/** One row that matters, or a fold of routine rows next to each other. */
export type DayItem = { type: 'row'; key: string; row: HistoryRow; tier: Tier } | { type: 'quiet'; key: string; rows: HistoryRow[] };

/** Poster size of the History rows: small log rows, medium, or large posters. */
export type ArtSize = 's' | 'm' | 'l';

export const ALL = 'all';

const download = ['release_chosen', 'release_chosen_by_hand', 'release_turned_down', 'turned_down_removed', 'release_held', 'held_removed', 'download_sent', 'download_queued', 'download_downloading', 'download_stalled', 'download_paused', 'download_finished', 'download_seeding', 'download_removed', 'download_changed'];

export const historyTabs: HistoryTab[] = [
	{ key: ALL, get label() { return say('downloads.ht.all.label'); }, get hint() { return say('downloads.ht.all.hint'); }, get empty() { return say('downloads.ht.all.empty'); }, subs: [] },
	{
		key: 'came',
		get label() { return say('downloads.ht.came.label'); },
		get hint() { return say('downloads.ht.came.hint'); },
		get empty() { return say('downloads.ht.came.empty'); },
		subs: [
			{ key: 'imported', get label() { return say('downloads.ht.came.imported.label'); }, get hint() { return say('downloads.ht.came.imported.hint'); }, kinds: ['import_done', 'import_partly_done'] },
			{ key: 'upgraded', get label() { return say('downloads.ht.came.upgraded.label'); }, get hint() { return say('downloads.ht.came.upgraded.hint'); }, kinds: ['file_replaced'] },
			{ key: 'found', get label() { return say('downloads.ht.came.found.label'); }, get hint() { return say('downloads.ht.came.found.hint'); }, kinds: ['adopted'] }
		]
	},
	{
		key: 'problems',
		get label() { return say('downloads.ht.problems.label'); },
		get hint() { return say('downloads.ht.problems.hint'); },
		get empty() { return say('downloads.ht.problems.empty'); },
		subs: [
			{ key: 'downloads', get label() { return say('downloads.ht.problems.downloads.label'); }, get hint() { return say('downloads.ht.problems.downloads.hint'); }, kinds: ['download_failed', 'download_vanished', 'download_not_removed', 'download_not_moved'] },
			{ key: 'imports', get label() { return say('downloads.ht.problems.imports.label'); }, get hint() { return say('downloads.ht.problems.imports.hint'); }, kinds: ['import_failed', 'import_rejected', 'file_hold_failed', 'free_space_low'] },
			{ key: 'decide', get label() { return say('downloads.ht.problems.decide.label'); }, get hint() { return say('downloads.ht.problems.decide.hint'); }, kinds: ['needs_decision', 'stopped_looking'] },
			{ key: 'health', get label() { return say('downloads.ht.problems.health.label'); }, get hint() { return say('downloads.ht.problems.health.hint'); }, kinds: ['health_broken', 'library_folder_*', 'file_missing', 'search_failed', 'alert_failed', 'media_server_failed'] },
			{ key: 'ai', get label() { return say('downloads.ht.problems.ai.label'); }, get hint() { return say('downloads.ht.problems.ai.hint'); }, kinds: ['ai_action_failed'] }
		]
	},
	{
		key: 'changes',
		get label() { return say('downloads.ht.changes.label'); },
		get hint() { return say('downloads.ht.changes.hint'); },
		get empty() { return say('downloads.ht.changes.empty'); },
		subs: [
			{ key: 'added', get label() { return say('downloads.ht.changes.added.label'); }, get hint() { return say('downloads.ht.changes.added.hint'); }, kinds: ['title_added', 'list_synced', 'version_changed', 'gate_opened'] },
			{ key: 'downloads', get label() { return say('downloads.ht.changes.downloads.label'); }, get hint() { return say('downloads.ht.changes.downloads.hint'); }, kinds: download },
			{ key: 'renamed', get label() { return say('downloads.ht.changes.renamed.label'); }, get hint() { return say('downloads.ht.changes.renamed.hint'); }, kinds: ['renamed', 'rename_planned'] },
			{ key: 'deleted', get label() { return say('downloads.ht.changes.deleted.label'); }, get hint() { return say('downloads.ht.changes.deleted.hint'); }, kinds: ['title_removed', 'episodes_removed', 'file_held', 'holding_cleaned'] },
			{ key: 'subtitles', get label() { return say('downloads.ht.changes.subtitles.label'); }, get hint() { return say('downloads.ht.changes.subtitles.hint'); }, kinds: ['subtitle_*'] },
			{ key: 'asks', get label() { return say('downloads.ht.changes.asks.label'); }, get hint() { return say('downloads.ht.changes.asks.hint'); }, kinds: ['ask_*', 'ai_ask_*'] },
			{ key: 'reports', get label() { return say('downloads.ht.changes.reports.label'); }, get hint() { return say('downloads.ht.changes.reports.hint'); }, kinds: ['report_*'] },
			{ key: 'system', get label() { return say('downloads.ht.changes.system.label'); }, get hint() { return say('downloads.ht.changes.system.hint'); }, kinds: ['sign*', 'api_key_*', 'settings_changed', 'secrets_imported', 'account_changed', 'backup_used', 'health_recovered'] }
		]
	},
	{
		key: 'routine',
		get label() { return say('downloads.ht.routine.label'); },
		get hint() { return say('downloads.ht.routine.hint'); },
		get empty() { return say('downloads.ht.routine.empty'); },
		subs: [
			{ key: 'searches', get label() { return say('downloads.ht.routine.searches.label'); }, get hint() { return say('downloads.ht.routine.searches.hint'); }, kinds: ['search_done', 'feed_checked'] },
			{ key: 'refreshes', get label() { return say('downloads.ht.routine.refreshes.label'); }, get hint() { return say('downloads.ht.routine.refreshes.hint'); }, kinds: ['title_refreshed', 'albums_changed', 'album_changed', 'episodes_changed', 'details_source_changed', 'season_changed', 'title_edited'] },
			{ key: 'scans', get label() { return say('downloads.ht.routine.scans.label'); }, get hint() { return say('downloads.ht.routine.scans.hint'); }, kinds: ['library_scanned', 'library_rescanned', 'folder_scanned', 'file_back', 'media_server_told'] }
		]
	}
];

/** What a tab (and sub-tab) asks the server for; undefined means every Event. */
export function tabKinds(tab: string, sub = ''): string[] | undefined {
	const t = historyTabs.find((x) => x.key === tab);
	if (!t || t.key === ALL) return undefined;
	const s = t.subs.find((x) => x.key === sub);
	return s ? [...s.kinds] : t.subs.flatMap((x) => x.kinds);
}

/** Reads &show=tab or tab.sub; unknown parts fall back to All with no sub-tab. */
export function showOf(v: string | null): { show: string; sub: string } {
	const [tab = '', sub = ''] = (v ?? '').split('.');
	const t = historyTabs.find((x) => x.key === tab);
	if (!t) return { show: ALL, sub: '' };
	return { show: t.key, sub: t.subs.some((s) => s.key === sub) ? sub : '' };
}

const matches = (kind: string, list: string[]) => list.some((k) => (k.endsWith('*') ? kind.startsWith(k.slice(0, -1)) : k === kind));

/** The big tab an Event kind is counted under; '' when only All has it. */
export function tabOf(kind: string): string {
	for (const t of historyTabs) for (const s of t.subs) if (matches(kind, s.kinds)) return t.key;
	return '';
}

/** Folds a page of Events into download rows, then into local days (newest first, order kept). */
export function byDay(events: EventRecord[]): DayGroup[] {
	const out: DayGroup[] = [];
	for (const row of groupDownloads(events)) {
		const k = dayKey(row.head.At);
		let d = out[out.length - 1];
		if (!d || d.key !== k) {
			d = { key: k, rows: [], tally: {} };
			out.push(d);
		}
		d.rows.push(row);
		const tab = rowTab(row);
		d.tally[tab] = (d.tally[tab] ?? 0) + 1;
	}
	return out;
}

/** A Library folder check that found the folder fine is news, not a problem. */
export const fineCheck = (e: Pick<EventRecord, 'Kind' | 'Data'>) => e.Kind === 'library_folder_health' && e.Data?.health === 'ok';

/** The tab a row counts under; a folded download by how far it got. */
export function rowTab(row: HistoryRow): string {
	if (fineCheck(row.head)) return 'changes';
	if (row.steps.some((s) => s.state === 'failed')) return 'problems';
	if (row.steps.length && row.steps[2]?.state === 'done') return 'came';
	if (row.steps.length) return 'changes';
	return tabOf(row.head.Kind) || 'changes';
}

/** What a day held, important first ("2 came in · 1 problem · 40 routine"). */
export function dayLine(tally: Record<string, number>): string {
	const parts: string[] = [];
	const n = (k: string) => tally[k] ?? 0;
	if (n('came')) parts.push(say('downloads.ht.dayCame', { n: n('came') }));
	if (n('problems')) parts.push(say(n('problems') === 1 ? 'downloads.ht.dayProblem.one' : 'downloads.ht.dayProblem.other', { n: n('problems') }));
	if (n('changes')) parts.push(say(n('changes') === 1 ? 'downloads.ht.dayChange.one' : 'downloads.ht.dayChange.other', { n: n('changes') }));
	if (n('routine')) parts.push(say('downloads.ht.dayRoutine', { n: n('routine') }));
	return parts.join(' · ');
}

/** Folds two or more adjacent routine rows into one quiet line. */
export function dayItems(rows: HistoryRow[]): DayItem[] {
	const out: DayItem[] = [];
	let run: HistoryRow[] = [];
	const flush = () => {
		if (run.length >= 2) out.push({ type: 'quiet', key: `q${run[0].key}`, rows: run });
		else for (const r of run) out.push({ type: 'row', key: r.key, row: r, tier: 'routine' });
		run = [];
	};
	for (const r of rows) {
		const tier = rowTier(r);
		if (tier === 'routine') {
			run.push(r);
			continue;
		}
		flush();
		out.push({ type: 'row', key: r.key, row: r, tier });
	}
	flush();
	return out;
}

const WHY = new Set(['Why', 'Error', 'Message']);

