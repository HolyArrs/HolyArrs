// History: the words and filters for the Event list (Downloads > History,
// the movie page and Home).
import type { EventRecord } from './types.ts';

export interface KindGroup {
	key: string;
	label: string;
	/** Event kinds; a kind ending in * is a prefix. */
	kinds: string[];
}

/** The kinds a person can filter by, in the order of the loop. */
export const kindGroups: KindGroup[] = [
	{ key: 'add', label: 'Added', kinds: ['title_added', 'adopted', 'list_synced', 'version_changed'] },
	{ key: 'search', label: 'Searched', kinds: ['search_done', 'search_failed', 'feed_checked'] },
	{ key: 'hold', label: 'Held', kinds: ['release_held', 'held_removed'] },
	{ key: 'choose', label: 'Chosen', kinds: ['release_chosen', 'release_chosen_by_hand', 'release_turned_down', 'turned_down_removed'] },
	{ key: 'grab', label: 'Sent to download', kinds: ['download_sent'] },
	{ key: 'download', label: 'Downloading', kinds: ['download_downloading', 'download_queued', 'download_stalled', 'download_paused', 'download_finished', 'download_seeding'] },
	{ key: 'fail', label: 'Failed', kinds: ['download_failed', 'download_vanished', 'import_failed', 'import_rejected', 'file_hold_failed'] },
	{ key: 'import', label: 'Imported', kinds: ['import_done', 'import_partly_done', 'file_replaced', 'free_space_low'] },
	{ key: 'rename', label: 'Renamed', kinds: ['renamed', 'rename_planned'] },
	{ key: 'delete', label: 'Deleted', kinds: ['title_removed', 'episodes_removed', 'file_held', 'holding_cleaned', 'download_removed', 'download_not_removed'] },
	{ key: 'subtitles', label: 'Subtitles', kinds: ['subtitle_*'] },
	{ key: 'scan', label: 'Scans', kinds: ['library_scanned', 'library_rescanned', 'folder_scanned', 'file_missing', 'file_back', 'library_folder_*', 'title_refreshed', 'episodes_changed', 'details_source_changed'] },
	{ key: 'decide', label: 'Needs a decision', kinds: ['needs_decision', 'stopped_looking'] },
	{ key: 'health', label: 'Health', kinds: ['health_*'] },
	{ key: 'wishlist', label: 'Wishlist', kinds: ['ask_*'] },
	{ key: 'links', label: 'Alerts and media servers', kinds: ['alert_failed', 'media_server_*', 'calendar_feed_key'] }
];

const exact: Record<string, string> = {};
const prefixes: [string, string][] = [];
for (const g of kindGroups) {
	for (const k of g.kinds) {
		if (k.endsWith('*')) prefixes.push([k.slice(0, -1), g.label]);
		else exact[k] = g.label;
	}
}

/** groupLabel names the group an Event kind belongs to ("Other" when none). */
export function groupLabel(kind: string): string {
	if (exact[kind]) return exact[kind];
	for (const [p, label] of prefixes) if (kind.startsWith(p)) return label;
	return 'Other';
}

/** tone colours a row by what happened. */
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
	from?: string;
	to?: string;
	limit: number;
	offset: number;
}

/** historyQuery turns the filters into the /events query string. */
export function historyQuery(f: HistoryFilter): string {
	const q = new URLSearchParams();
	if (f.episode) q.set('episode', String(f.episode));
	else if (f.title) q.set('title', String(f.title));
	const kinds = kindGroups.filter((g) => f.groups.includes(g.key)).flatMap((g) => g.kinds);
	if (kinds.length) q.set('kind', kinds.join(','));
	if (f.from) q.set('from', f.from);
	if (f.to) q.set('to', f.to);
	q.set('limit', String(f.limit));
	if (f.offset) q.set('offset', String(f.offset));
	return q.toString();
}

/** readFilter reads the filters from the page address (so a filtered view can be linked). */
export function readFilter(params: { get(name: string): string | null }, limit = 50): HistoryFilter {
	const title = Number(params.get('title'));
	const episode = Number(params.get('episode'));
	const day = (v: string | null) => (v && /^\d{4}-\d{2}-\d{2}$/.test(v) ? v : undefined);
	const groups = (params.get('kinds') ?? '').split(',').filter((k) => kindGroups.some((g) => g.key === k));
	const offset = Math.max(0, Number(params.get('offset')) || 0);
	return {
		title: Number.isInteger(title) && title > 0 ? title : undefined,
		episode: Number.isInteger(episode) && episode > 0 ? episode : undefined,
		groups, from: day(params.get('from')), to: day(params.get('to')), limit, offset };
}

/** filterParams is the page address for the filters. */
export function filterParams(f: HistoryFilter): string {
	const q = new URLSearchParams();
	if (f.title) q.set('title', String(f.title));
	if (f.episode) q.set('episode', String(f.episode));
	if (f.groups.length) q.set('kinds', f.groups.join(','));
	if (f.from) q.set('from', f.from);
	if (f.to) q.set('to', f.to);
	if (f.offset) q.set('offset', String(f.offset));
	const s = q.toString();
	return s ? '?' + s : '';
}

const detailWords: [string, string][] = [
	['reason', 'Why'],
	['release', 'Release'],
	['downloader', 'Downloader'],
	['save_path', 'Downloaded to'],
	['from', 'From'],
	['to', 'To'],
	['folder', 'Folder'],
	['path', 'Path'],
	['message', 'Message'],
	['error', 'Error']
];

/** details lists the reasons and paths an Event carries, as label and text. */
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
			if (p && typeof p === 'object' && typeof (p as { to?: unknown }).to === 'string') out.push(['Placed at', (p as { to: string }).to]);
			else if (typeof p === 'string') out.push(['Folder', p]);
		}
	}
	return out;
}

/** pageLine says which rows of how many are shown. */
export function pageLine(total: number, offset: number, shown: number): string {
	if (total === 0) return 'No Events';
	return `${offset + 1} to ${offset + shown} of ${total}`;
}
