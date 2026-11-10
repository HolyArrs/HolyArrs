import { t } from '#lib/core/i18n/i18n.svelte.ts';
import { controllable, pausable, retryable } from './usenet.ts';
import { protocolOf } from './size.ts';

/** One entry in a row's ⋯ menu; the page maps each key to its handler. */
export type RowItem = 'pause' | 'resume' | 'retry' | 'up' | 'down' | 'top' | 'bottom' | 'priority' | 'library' | 'remove';

export type Priority = (typeof PRIORITIES)[number]['key'];

type RowBits = Parameters<typeof retryable>[0] & {
	can_act?: boolean;
	client_locked?: boolean;
	state?: string;
	protocol?: string;

	downloader_kind?: string;
	downloader_id?: number;
	hash?: string;
	client_item?: boolean;
	title_id?: number;
	linked?: boolean;
	outside?: boolean;
};

/** What the details pane's ⋯ adds beyond the row's list. */
export type PaneExtra = 'recheck' | 'reannounce' | 'force' | 'steer' | 'fail';

/** The Usenet priorities the server takes, in menu order. */
export const PRIORITIES = [
	{ key: 'low', get label() { return t('downloads.priority.low'); } },
	{ key: 'normal', get label() { return t('downloads.priority.normal'); } },
	{ key: 'high', get label() { return t('downloads.priority.high'); } },
	{ key: 'force', get label() { return t('downloads.priority.force'); } }
] as const;

// A Usenet job still in a SABnzbd or NZBGet queue takes order and priority.
const QUEUED = new Set(['waiting', 'downloading', 'paused', 'stalled']);

/** Whether a row is a Usenet job still in its client's queue. */
export function usenetQueued(r: { protocol?: string; downloader_kind?: string; phase?: string; downloader_id?: number; hash?: string }): boolean {
	return r.protocol === 'usenet' && (r.downloader_kind === 'sabnzbd' || r.downloader_kind === 'nzbget') && !!r.downloader_id && !!r.hash && QUEUED.has(r.phase ?? '');
}

/** Lists what a row's ⋯ menu offers, in order. A read-only row (can_act false) gets only Open in Library. Queue moves are qBittorrent torrents (the client bulk call) and SABnzbd or NZBGet jobs. */
/** HolyArrs may change this row in its Downloader, not only its own records. */
export const clientOk = (r: { can_act?: boolean; client_locked?: boolean }) => !!r.can_act && !r.client_locked;

export function rowItems(r: RowBits): RowItem[] {
	const out: RowItem[] = [];
	const lib = !!r.title_id && (!!r.linked || !r.outside);
	if (clientOk(r)) {
		if (pausable(r)) out.push(r.state === 'paused' ? 'resume' : 'pause');
		if (retryable(r)) out.push('retry');
		if (!!r.downloader_id && !!r.hash && r.protocol === 'torrent' && (!r.downloader_kind || r.downloader_kind === 'qbittorrent')) out.push('up', 'down', 'top', 'bottom');
		else if (usenetQueued(r)) out.push('up', 'down', 'top', 'bottom', 'priority');
	}
	if (lib) out.push('library');
	if (r.can_act) out.push('remove');
	return out;
}

/** A torrent's checks (Check again, Ask trackers again), force start and its sheets when the page can steer it, and Mark failed when the page allows it. A read-only row gets none. */
export function paneExtras(r: RowBits, opts: { steer: boolean; fail: boolean }): PaneExtra[] {
	if (!r.can_act) return [];
	const out: PaneExtra[] = [];
	const torrent = protocolOf(r) === 'torrent';
	const client = clientOk(r);
	if (client && torrent && controllable(r.downloader_kind)) out.push('recheck', 'reannounce');
	// Force start and the sheets are qBittorrent calls; Transmission and Deluge refuse them.
	if (client && torrent && controllable(r.downloader_kind) && opts.steer && !!r.downloader_id && !!r.hash) out.push('force', 'steer');
	if (opts.fail) out.push('fail');
	return out;
}
