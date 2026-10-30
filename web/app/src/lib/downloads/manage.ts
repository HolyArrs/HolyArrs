// Helpers for steering a torrent Downloader by hand from the Downloads page:
// the Add sheet, the selection, the seeding rules and the row actions. Pure,
// so they can be tested.

/** One add's answer from the server. */
export interface AddOutcome {
	added: number;
	failed: number;
	pending: number;
	duplicates: number;
	bad?: string[];
	/** Magnet addresses left out because they point at this machine or the local network. */
	dropped?: number;
}

/** splitLinks reads the Add box: one link per line; blank lines are skipped. A line that is no magnet, http(s) link or info hash is bad. */
export function splitLinks(text: string): { links: string[]; bad: string[] } {
	const links: string[] = [];
	const bad: string[] = [];
	for (const raw of text.split(/\r?\n/)) {
		const l = raw.trim();
		if (!l) continue;
		if (/^magnet:\?/i.test(l) || /^https?:\/\/\S+$/i.test(l) || /^[0-9a-f]{40}$/i.test(l)) links.push(l);
		else bad.push(l);
	}
	return { links, bad };
}

const n = (k: number, one: string, many = `${one}s`) => `${k} ${k === 1 ? one : many}`;

/** addWords says how an add went, in one line, and whether it is good news. */
export function addWords(o: AddOutcome): { text: string; tone: 'ok' | 'info' | 'fail' } {
	const parts: string[] = [];
	if (o.added) parts.push(`${n(o.added, 'torrent')} added`);
	if (o.pending) parts.push(`${o.pending} still being fetched`);
	if (o.duplicates) parts.push(`${o.duplicates} already there`);
	if (o.failed) parts.push(`${o.failed} failed`);
	if (o.bad?.length) parts.push(`could not add: ${o.bad.join(', ')}`);
	if (o.dropped) parts.push(`${n(o.dropped, 'local address', 'local addresses')} left out of the magnet links`);
	const text = parts.length ? upper(parts.join(' · ')) + '.' : 'Nothing was added.';
	const tone = o.failed || o.bad?.length ? (o.added || o.pending ? 'info' : 'fail') : o.added || o.pending ? 'ok' : 'info';
	return { text, tone };
}

const upper = (s: string) => s.charAt(0).toUpperCase() + s.slice(1);

/** isTorrentFile keeps dropped files that look like .torrent files. */
export const isTorrentFile = (f: { name: string; type?: string }) => /\.torrent$/i.test(f.name) || f.type === 'application/x-bittorrent';

/** The actions the selection bar and the row menu send in one call per Downloader. */
export type BulkAction = 'start' | 'pause' | 'recheck' | 'reannounce' | 'top' | 'bottom' | 'up' | 'down' | 'force' | 'unforce';

/** pickRange is every id from a to b in list order, both ends included. */
export function pickRange(ids: number[], a: number, b: number): number[] {
	const i = ids.indexOf(a);
	const j = ids.indexOf(b);
	if (j < 0) return [];
	if (i < 0) return [b];
	return ids.slice(Math.min(i, j), Math.max(i, j) + 1);
}

type Addressed = { downloader_id?: number; hash?: string; protocol?: string };
/** byClient groups rows a torrent Downloader can act on by Downloader id; rows with no hash (Usenet, not sent yet) are left out. */
export function byClient<T extends Addressed>(rows: T[]): Map<number, T[]> {
	const m = new Map<number, T[]>();
	for (const r of rows) {
		if (!r.downloader_id || !r.hash || (r.protocol && r.protocol !== 'torrent')) continue;
		m.set(r.downloader_id, [...(m.get(r.downloader_id) ?? []), r]);
	}
	return m;
}

/** BULK_DONE is the toast after a selection action. */
export const BULK_DONE: Record<BulkAction, string> = {
	start: 'Started', pause: 'Paused', recheck: 'Checking again', reannounce: 'Asked the trackers again', top: 'Moved to the top', bottom: 'Moved to the bottom',
	up: 'Moved up', down: 'Moved down', force: 'Force started', unforce: 'Back in the queue'
};
