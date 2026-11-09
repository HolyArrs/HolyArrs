// The words and filters for the Event list (Downloads > History, the movie page and Home).
import type { EventRecord } from '../core/types.ts';

export interface KindGroup {
	key: string;
	label: string;
	/** Event kinds; a kind ending in * is a prefix. */
	kinds: string[];
}

/** The steps of one download, in order. */
export type StepKey = 'sent' | 'downloaded' | 'imported';

export type StepState = 'done' | 'failed' | 'waiting';

export interface Step {
	key: StepKey;
	label: string;
	state: StepState;
}

/** One Event, or every step of one download folded together (newest first). */
export interface HistoryRow {
	key: string;
	/** The newest Event; its title, time and kind lead the row. */
	head: EventRecord;
	events: EventRecord[];
	/** A download's step line; empty for a single Event. */
	steps: Step[];
}

/** The kinds a person can filter by, in the order of the loop. */
export const kindGroups: KindGroup[] = [
	{ key: 'add', label: 'Added', kinds: ['title_added', 'adopted', 'list_synced', 'version_changed'] },
	{ key: 'search', label: 'Searched', kinds: ['search_done', 'search_failed', 'feed_checked'] },
	{ key: 'hold', label: 'Held', kinds: ['release_held', 'held_removed'] },
	{ key: 'choose', label: 'Chosen', kinds: ['release_chosen', 'release_chosen_by_hand', 'release_turned_down', 'turned_down_removed'] },
	{ key: 'grab', label: 'Sent to download', kinds: ['download_sent'] },
	{ key: 'download', label: 'Downloading', kinds: ['download_downloading', 'download_queued', 'download_stalled', 'download_paused', 'download_finished', 'download_seeding', 'download_changed'] },
	{ key: 'fail', label: 'Failed', kinds: ['download_failed', 'download_vanished', 'import_failed', 'import_rejected', 'file_hold_failed'] },
	{ key: 'import', label: 'Imported', kinds: ['import_done', 'import_partly_done', 'file_replaced', 'free_space_low'] },
	{ key: 'rename', label: 'Renamed', kinds: ['renamed', 'rename_planned'] },
	{ key: 'delete', label: 'Deleted', kinds: ['title_removed', 'episodes_removed', 'file_held', 'holding_cleaned', 'download_removed', 'download_not_removed'] },
	{ key: 'subtitles', label: 'Subtitles', kinds: ['subtitle_*'] },
	{ key: 'scan', label: 'Scans', kinds: ['library_scanned', 'library_rescanned', 'folder_scanned', 'file_missing', 'file_back', 'library_folder_*', 'title_refreshed', 'episodes_changed', 'details_source_changed'] },
	{ key: 'decide', label: 'Needs a decision', kinds: ['needs_decision', 'stopped_looking'] },
	{ key: 'health', label: 'Health', kinds: ['health_*'] },
	{ key: 'wishlist', label: 'Requests', kinds: ['ask_*'] },
	{ key: 'reports', label: 'Reported problems', kinds: ['report_*'] },
	{ key: 'links', label: 'Alerts and media servers', kinds: ['alert_failed', 'media_server_*'] },
	{ key: 'ai', label: 'AI', kinds: ['ai_*'] },
	{ key: 'system', label: 'System', kinds: ['sign*', 'api_key_*', 'settings_changed'] }
];

const exact: Record<string, string> = {};
const prefixes: [string, string][] = [];

for (const g of kindGroups) {
	for (const k of g.kinds) {
		if (k.endsWith('*')) prefixes.push([k.slice(0, -1), g.label]);
		else exact[k] = g.label;
	}
}

/** The group an Event kind belongs to; "Other" when none. */
export function groupLabel(kind: string): string {
	if (exact[kind]) return exact[kind];
	for (const [p, label] of prefixes) if (kind.startsWith(p)) return label;
	return 'Other';
}

/** Colours a row by what happened. */
export function kindTone(kind: string): 'ok' | 'fail' | 'wait' | '' {
	if (kind === 'health_recovered' || kind === 'import_done' || kind === 'file_back' || kind === 'media_server_told' || kind === 'subtitle_saved' || kind === 'subtitle_upgraded') return 'ok';
	if (/failed|rejected|vanished|missing|unreachable|broken|low|not_removed/.test(kind) || kind === 'needs_decision') return 'fail';
	if (kind === 'release_held' || kind === 'import_partly_done' || kind === 'download_stalled' || kind === 'subtitle_none_good_enough' || kind === 'subtitle_site_paused') return 'wait';
	return '';
}

export interface HistoryFilter {
	title?: number;
	/** One episode of a show (its id): its own rows and its packs'. */
	episode?: number;
	groups: string[];
	/** One Source's grabs and pauses (owners and admins). */
	source?: number;
	/** Show the routine feed checks (hidden by default). */
	feed?: boolean;
	/** Ask the server to hide feed checks unless `feed` is on. */
	hideFeed?: boolean;
	from?: string;
	to?: string;
	limit: number;
	offset: number;
}

/** Lets an async caller ignore work superseded by a newer request. */
export function createLatestOnly(): () => () => boolean {
	let latest = 0;
	return () => {
		const request = ++latest;
		return () => request === latest;
	};
}

/** Explains an invalid inclusive date range. */
export function dateRangeProblem(f: Pick<HistoryFilter, 'from' | 'to'>): string {
	return f.from && f.to && f.from > f.to ? 'From date must be on or before To date.' : '';
}

/** Turns the filters into the /events query string. */
export function historyQuery(f: HistoryFilter): string {
	const problem = dateRangeProblem(f);
	if (problem) throw new Error(problem);
	const q = new URLSearchParams();
	if (f.episode) q.set('episode', String(f.episode));
	else if (f.title) q.set('title', String(f.title));
	const kinds = kindGroups.filter((g) => f.groups.includes(g.key)).flatMap((g) => g.kinds);
	if (kinds.length) q.set('kind', kinds.join(','));
	if (f.source) q.set('source', String(f.source));
	if (f.feed) q.set('feed', '1');
	else if (f.hideFeed) q.set('feed', '0');
	if (f.from) q.set('from', f.from);
	if (f.to) q.set('to', f.to);
	q.set('limit', String(f.limit));
	if (f.offset) q.set('offset', String(f.offset));
	return q.toString();
}

/** Reads the filters from the page address, so a filtered view can be linked. */
export function readFilter(params: { get(name: string): string | null }, limit = 50): HistoryFilter {
	const title = Number(params.get('title'));
	const episode = Number(params.get('episode'));
	const day = (v: string | null) => {
		if (!v || !/^\d{4}-\d{2}-\d{2}$/.test(v)) return undefined;
		const parsed = new Date(v + 'T00:00:00Z');
		return Number.isFinite(parsed.getTime()) && parsed.toISOString().slice(0, 10) === v ? v : undefined;
	};
	const groups = (params.get('kinds') ?? '').split(',').filter((k) => kindGroups.some((g) => g.key === k));
	const offset = Math.max(0, Number(params.get('offset')) || 0);
	const source = Number(params.get('source'));
	return {
		source: Number.isInteger(source) && source > 0 ? source : undefined,
		feed: params.get('feed') === '1' || undefined,
		title: Number.isInteger(title) && title > 0 ? title : undefined,
		episode: Number.isInteger(episode) && episode > 0 ? episode : undefined,
		groups, from: day(params.get('from')), to: day(params.get('to')), limit, offset };
}

const detailWords: [string, string][] = [
	['reason', 'Why'],
	['release', 'Release'],
	['downloader', 'Downloader'],
	['save_path', 'Folder'],
	['from', 'From'],
	['to', 'To'],
	['folder', 'Folder'],
	['path', 'Folder'],
	['message', 'Message'],
	['error', 'Error']
];

/** Lists the reasons and paths an Event carries, as label and text. */
export function details(e: Pick<EventRecord, 'Data'>): [string, string][] {
	const d = e.Data ?? {};
	const out: [string, string][] = [];
	for (const [k, label] of detailWords) {
		const v = d[k];
		if (typeof v === 'string' && v) out.push([label, v]);
	}
	const paths = d.paths;
	if (Array.isArray(paths)) {
		for (const p of paths) {
			if (p && typeof p === 'object' && typeof (p as { to?: unknown }).to === 'string') out.push(['File', (p as { to: string }).to]);
			else if (typeof p === 'string') out.push(['Folder', p]);
		}
	}
	return out;
}

/** Which rows of how many are shown. */
export function pageLine(total: number, offset: number, shown: number): string {
	if (total === 0) return 'No Events';
	return `${offset + 1} to ${offset + shown} of ${total}`;
}

const downloadGroups = new Set(['grab', 'download', 'fail', 'import']);
const groupOfKind = (kind: string) => kindGroups.find((g) => g.kinds.includes(kind))?.key ?? '';

/** Whether an Event is a step of a download (sent, downloading, failed, imported). */
export function isDownloadStep(e: Pick<EventRecord, 'Kind' | 'DownloadID'>): boolean {
	return !!e.DownloadID && downloadGroups.has(groupOfKind(e.Kind));
}

/** Which of sent, downloaded and imported a download reached, and where it failed. */
export function downloadSteps(events: Pick<EventRecord, 'Kind'>[]): Step[] {
	const kinds = new Set(events.map((e) => e.Kind));
	const has = (...k: string[]) => k.some((x) => kinds.has(x));
	const downloadFailed = has('download_failed', 'download_vanished');
	const importFailed = has('import_failed', 'import_rejected', 'file_hold_failed', 'free_space_low');
	const imported = has('import_done', 'import_partly_done', 'file_replaced');
	const downloaded = imported || importFailed || has('download_finished', 'download_seeding');
	const sent = downloaded || downloadFailed || events.some((e) => groupOfKind(e.Kind) === 'grab' || groupOfKind(e.Kind) === 'download');
	const state = (done: boolean, failed: boolean): StepState => (failed ? 'failed' : done ? 'done' : 'waiting');
	return [
		{ key: 'sent', label: 'Sent', state: state(sent, false) },
		{ key: 'downloaded', label: 'Downloaded', state: state(downloaded, downloadFailed && !downloaded) },
		{ key: 'imported', label: 'Imported', state: state(imported, importFailed && !imported) }
	];
}

/** Folds the Events of one download (sent, downloading, finished, imported or failed) into one row, placed where its newest Event is. The pick that started it and a decision waiting on it join the same row. */
export function groupDownloads(events: EventRecord[]): HistoryRow[] {
	const numOf = (v: unknown) => (typeof v === 'number' && v > 0 ? v : 0);
	// Which download each release and each stuck Slot belongs to, read from the download's own steps.
	const byRelease = new Map<number, number>();
	const stuckSlot = new Map<number, number>();
	for (const e of events) {
		if (!isDownloadStep(e)) continue;
		const rel = numOf(e.Data?.release_id);
		if (rel && !byRelease.has(rel)) byRelease.set(rel, e.DownloadID!);
		if (e.SlotID && /^import_(failed|rejected)$/.test(e.Kind) && !stuckSlot.has(e.SlotID)) stuckSlot.set(e.SlotID, e.DownloadID!);
	}
	const known = new Set(byRelease.values());
	for (const e of events) if (isDownloadStep(e)) known.add(e.DownloadID!);
	const downloadOf = (e: EventRecord): number => {
		if (isDownloadStep(e)) return e.DownloadID!;
		if (e.Kind === 'release_chosen' || e.Kind === 'release_chosen_by_hand') return byRelease.get(numOf(e.Data?.release_id)) ?? 0;
		if (e.Kind === 'needs_decision' || e.Kind === 'stopped_looking') {
			const id = e.DownloadID || numOf(e.Data?.download_id);
			if (id && known.has(id)) return id;
			return e.Kind === 'needs_decision' && e.SlotID ? (stuckSlot.get(e.SlotID) ?? 0) : 0;
		}
		return 0;
	};
	const rows: HistoryRow[] = [];
	const byDownload = new Map<number, HistoryRow>();
	for (const e of events) {
		const id = downloadOf(e);
		if (!id) {
			rows.push({ key: `e${e.ID}`, head: e, events: [e], steps: [] });
			continue;
		}
		const row = byDownload.get(id);
		if (row) {
			row.events.push(e);
			continue;
		}
		const fresh: HistoryRow = { key: `d${id}`, head: e, events: [e], steps: [] };
		byDownload.set(id, fresh);
		rows.push(fresh);
	}
	for (const row of byDownload.values()) row.steps = downloadSteps(row.events);
	return rows;
}
