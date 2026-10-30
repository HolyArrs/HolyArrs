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
	if (o.bad?.length) parts.push(`not a torrent: ${o.bad.join(', ')}`);
	const text = parts.length ? upper(parts.join(' · ')) + '.' : 'Nothing was added.';
	const tone = o.failed || o.bad?.length ? (o.added || o.pending ? 'info' : 'fail') : o.added || o.pending ? 'ok' : 'info';
	return { text, tone };
}

const upper = (s: string) => s.charAt(0).toUpperCase() + s.slice(1);

/** isTorrentFile keeps dropped files that look like .torrent files. */
export const isTorrentFile = (f: { name: string; type?: string }) => /\.torrent$/i.test(f.name) || f.type === 'application/x-bittorrent';
