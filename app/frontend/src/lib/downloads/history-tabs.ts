// History tabs shared with the Downloads table. Five big tabs answer what a person opens History for: what came in, what went wrong, what changed, what ran in the background.
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
	{ key: ALL, label: 'All', hint: 'Every Event, newest first.', empty: 'No history yet.', subs: [] },
	{
		key: 'came',
		label: 'Came in',
		hint: 'Files that reached the Library: imported, upgraded or found on disk.',
		empty: 'Nothing came in.',
		subs: [
			{ key: 'imported', label: 'Imported', hint: 'Downloads moved into the Library.', kinds: ['import_done', 'import_partly_done'] },
			{ key: 'upgraded', label: 'Upgraded', hint: 'A better file replaced the one you had.', kinds: ['file_replaced'] },
			{ key: 'found', label: 'Found on disk', hint: 'Files already in a Library folder that HolyArrs took on.', kinds: ['adopted'] }
		]
	},
	{
		key: 'problems',
		label: 'Problems',
		hint: 'Failed downloads and imports, decisions waiting for you, and health.',
		empty: 'Nothing went wrong.',
		subs: [
			{ key: 'downloads', label: 'Downloads', hint: 'Downloads that failed or went missing from the Downloader.', kinds: ['download_failed', 'download_vanished', 'download_not_removed'] },
			{ key: 'imports', label: 'Imports', hint: 'Finished downloads HolyArrs could not move into the Library.', kinds: ['import_failed', 'import_rejected', 'file_hold_failed', 'free_space_low'] },
			{ key: 'decide', label: 'Needs a decision', hint: 'HolyArrs stopped and waits for you to pick.', kinds: ['needs_decision', 'stopped_looking'] },
			{ key: 'health', label: 'Health', hint: 'Checks that broke, folders out of reach, missing files, alerts that failed.', kinds: ['health_broken', 'library_folder_*', 'file_missing', 'search_failed', 'alert_failed', 'media_server_failed'] },
			{ key: 'ai', label: 'AI apps', hint: 'Things an AI app asked for that HolyArrs did not do.', kinds: ['ai_action_failed'] }
		]
	},
	{
		key: 'changes',
		label: 'Changes',
		hint: 'What was added, sent to download, renamed or deleted, subtitles and asks.',
		empty: 'Nothing changed.',
		subs: [
			{ key: 'added', label: 'Added', hint: 'Movies, shows and artists added, by hand, from a list or as they came out.', kinds: ['title_added', 'list_synced', 'version_changed', 'gate_opened'] },
			{ key: 'downloads', label: 'Downloads', hint: 'Releases chosen, held or turned down, and their downloads.', kinds: download },
			{ key: 'renamed', label: 'Renamed', hint: 'Files and folders renamed.', kinds: ['renamed', 'rename_planned'] },
			{ key: 'deleted', label: 'Deleted', hint: 'Titles, episodes and files deleted or held.', kinds: ['title_removed', 'episodes_removed', 'file_held', 'holding_cleaned'] },
			{ key: 'subtitles', label: 'Subtitles', hint: 'Subtitles saved, upgraded, synced or not found.', kinds: ['subtitle_*'] },
			{ key: 'asks', label: 'Asks', hint: 'Asks made, approved or turned down, by people or AI apps.', kinds: ['ask_*', 'ai_ask_*'] },
			{ key: 'reports', label: 'Reports', hint: 'Problems people reported with titles, fixed or opened again.', kinds: ['report_*'] },
			{ key: 'system', label: 'System', hint: 'Sign-ins, API keys, account and settings changes.', kinds: ['sign*', 'api_key_*', 'settings_changed', 'secrets_imported', 'account_changed', 'backup_used', 'health_recovered'] }
		]
	},
	{
		key: 'routine',
		label: 'Routine',
		hint: 'Searches, refreshed details and folder scans HolyArrs runs on its own.',
		empty: 'No routine work yet.',
		subs: [
			{ key: 'searches', label: 'Searches', hint: 'Searches for Releases and feed checks.', kinds: ['search_done', 'feed_checked'] },
			{ key: 'refreshes', label: 'Refreshed', hint: 'Details, albums and episodes read again from their source.', kinds: ['title_refreshed', 'albums_changed', 'album_changed', 'episodes_changed', 'details_source_changed', 'season_changed', 'title_edited'] },
			{ key: 'scans', label: 'Scans', hint: 'Library folders looked at again.', kinds: ['library_scanned', 'library_rescanned', 'folder_scanned', 'file_back', 'media_server_told'] }
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
	if (n('came')) parts.push(`${n('came')} came in`);
	if (n('problems')) parts.push(`${n('problems')} ${n('problems') === 1 ? 'problem' : 'problems'}`);
	if (n('changes')) parts.push(`${n('changes')} ${n('changes') === 1 ? 'change' : 'changes'}`);
	if (n('routine')) parts.push(`${n('routine')} routine`);
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

