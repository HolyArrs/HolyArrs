import { pausable, retryable } from './usenet.ts';

/** One entry in a row's ⋯ menu; the page maps each key to its handler. */
export type RowItem = 'pause' | 'resume' | 'retry' | 'up' | 'down' | 'top' | 'bottom' | 'library' | 'remove';

type RowBits = Parameters<typeof retryable>[0] & {
	can_act?: boolean;
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

/** rowItems lists what a row's ⋯ menu offers, in order. A read-only row
 *  (can_act false) gets only Open in Library: nothing that changes the client.
 *  Queue moves are qBittorrent torrents only (the client bulk call); neither
 *  SABnzbd nor NZBGet takes a job priority from HolyArrs yet. */
export function rowItems(r: RowBits): RowItem[] {
	const out: RowItem[] = [];
	const lib = !!r.title_id && (!!r.linked || !r.outside);
	if (r.can_act) {
		if (pausable(r)) out.push(r.state === 'paused' ? 'resume' : 'pause');
		if (retryable(r)) out.push('retry');
		if (!!r.downloader_id && !!r.hash && r.protocol === 'torrent' && (!r.downloader_kind || r.downloader_kind === 'qbittorrent')) out.push('up', 'down', 'top', 'bottom');
	}
	if (lib) out.push('library');
	if (r.can_act) out.push('remove');
	return out;
}
