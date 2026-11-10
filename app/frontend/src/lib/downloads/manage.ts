// Helpers for steering a torrent Downloader by hand from the Downloads page: the Add sheet, the selection, the seeding rules and the row actions. Pure, so they can be tested.
import { t } from '#lib/core/i18n/i18n.svelte.ts';

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

/** Actions the selection bar and row menu send in one call per Downloader. */
export type BulkAction = 'start' | 'pause' | 'recheck' | 'reannounce' | 'top' | 'bottom' | 'up' | 'down' | 'force' | 'unforce';

type Addressed = { downloader_id?: number; hash?: string; protocol?: string };

/** A torrent's own seeding rules: -2 follows the Downloader's rule, -1 is no limit. */
export interface ShareLimits {
	ratio: number;
	seeding_minutes: number;
	inactive_minutes: number;
	action: '' | 'stop' | 'remove' | 'remove_files' | 'super_seed';
}

/** One limit in the Seeding rules sheet: the Downloader's rule, none, or a value. */
export type LimitMode = 'global' | 'none' | 'set';

/** What the steering sheet edits. */
export type SteerMode = 'seed' | 'location' | 'speed';

/** Tracker filter: '' is all, host:<name> one tracker host, p:<problem> a problem. */
export type TrackerProblem = 'error' | 'warning' | 'announce';

type Tracked = { tracker?: string; tracker_problem?: TrackerProblem };

/** What the toolbar's Start, Pause and Remove work on: the ticked rows if any, else the open row. */
export type ToolTarget = { protocol?: string; downloader_kind?: string; state?: string; can_act: boolean; client_locked?: boolean; phase?: string };

/** Reads the Add box, one link per line; blank lines are skipped. A line that is no magnet, http(s) link or info hash is bad. */
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


/** How an add went (the main result, then the rest as a body) and whether it is good news. */
export function addWords(o: AddOutcome): { text: string; body?: string; tone: 'ok' | 'info' | 'fail' } {
	const parts: string[] = [];
	if (o.added) parts.push(t(o.added === 1 ? 'downloads.add.addedOne' : 'downloads.add.added', { count: o.added }));
	if (o.pending) parts.push(t('downloads.add.pending', { count: o.pending }));
	if (o.duplicates) parts.push(t('downloads.add.duplicates', { count: o.duplicates }));
	if (o.failed) parts.push(t('downloads.add.failed', { count: o.failed }));
	if (o.bad?.length) parts.push(t('downloads.add.bad', { list: o.bad.join(', ') }));
	if (o.dropped) parts.push(t(o.dropped === 1 ? 'downloads.add.dropped.one' : 'downloads.add.dropped.other', { count: o.dropped }));
	const text = parts.length ? upper(parts[0]) : t('downloads.add.nothing');
	const tone = o.failed || o.bad?.length ? (o.added || o.pending ? 'info' : 'fail') : o.added || o.pending ? 'ok' : 'info';
	return parts.length > 1 ? { text, body: parts.slice(1).join(' · '), tone } : { text, tone };
}

const upper = (s: string) => s.charAt(0).toUpperCase() + s.slice(1);

/** Keeps dropped files that look like .torrent files. */
export const isTorrentFile = (f: { name: string; type?: string }) => /\.torrent$/i.test(f.name) || f.type === 'application/x-bittorrent';

/** Every id from a to b in list order, both ends included. */
export function pickRange(ids: number[], a: number, b: number): number[] {
	const i = ids.indexOf(a);
	const j = ids.indexOf(b);
	if (j < 0) return [];
	if (i < 0) return [b];
	return ids.slice(Math.min(i, j), Math.max(i, j) + 1);
}

/** Groups rows a torrent Downloader can act on by Downloader id; rows with no hash (Usenet, not sent yet) are left out. */
export function byClient<T extends Addressed>(rows: T[]): Map<number, T[]> {
	const m = new Map<number, T[]>();
	for (const r of rows) {
		if (!r.downloader_id || !r.hash || (r.protocol && r.protocol !== 'torrent')) continue;
		m.set(r.downloader_id, [...(m.get(r.downloader_id) ?? []), r]);
	}
	return m;
}

/** The toast after a selection action: "3 downloads paused". */
export const BULK_DONE: Record<BulkAction, string> = {
	get start() { return t('downloads.bulk.start'); }, get pause() { return t('downloads.bulk.pause'); }, get recheck() { return t('downloads.bulk.recheck'); }, get reannounce() { return t('downloads.bulk.reannounce'); }, get top() { return t('downloads.bulk.top'); }, get bottom() { return t('downloads.bulk.bottom'); },
	get up() { return t('downloads.bulk.up'); }, get down() { return t('downloads.bulk.down'); }, get force() { return t('downloads.bulk.force'); }, get unforce() { return t('downloads.bulk.unforce'); }
};

export const modeOf = (v: number): LimitMode => (v === -2 ? 'global' : v < 0 ? 'none' : 'set');
export const valueOf = (mode: LimitMode, v: number) => (mode === 'global' ? -2 : mode === 'none' ? -1 : v);

const minutesWords = (m: number) => {
	const [k, unit] = m % 1440 === 0 ? [m / 1440, 'day'] : m % 60 === 0 ? [m / 60, 'hour'] : [m, 'minute'];
	return t(`downloads.limits.${unit}.${k === 1 ? 'one' : 'other'}`, { count: k });
};
const ACTION_WORD: Record<ShareLimits['action'], string> = {
	'': '',
	get stop() { return t('downloads.limits.thenStop'); },
	get remove() { return t('downloads.limits.thenRemove'); },
	get remove_files() { return t('downloads.limits.thenRemoveFiles'); },
	get super_seed() { return t('downloads.limits.thenSuper'); }
};

/** A torrent's seeding rules in one line, for Info. */
export function limitsWords(l: ShareLimits | undefined): string {
	if (!l) return '–';
	const one = (v: number, set: (v: number) => string) => (v === -2 ? '' : v < 0 ? 'none' : set(v));
	const parts = [
		[one(l.ratio, (v) => t('downloads.limits.ratio', { v })), 'ratio'],
		[one(l.seeding_minutes, minutesWords), 'seeding'],
		[one(l.inactive_minutes, (v) => t('downloads.limits.idle', { time: minutesWords(v) })), 'idle']
	]
		.filter(([w]) => w)
		.map(([w, k]) => (w === 'none' ? t(k === 'ratio' ? 'downloads.limits.noRatio' : k === 'seeding' ? 'downloads.limits.noSeeding' : 'downloads.limits.noIdle') : w));
	const base = parts.length ? upper(parts.join(' · ')) : t('downloads.limits.rules');
	return ACTION_WORD[l.action] ? t('downloads.limits.baseThen', { base, action: ACTION_WORD[l.action] }) : base;
}

/** What is wrong with typed values, or ''. */
export function checkLimits(l: ShareLimits): string {
	if (l.ratio >= 0 && !(l.ratio <= 9998)) return t('downloads.limits.ratioRange');
	for (const m of [l.seeding_minutes, l.inactive_minutes]) if (m >= 0 && !(Number.isInteger(m) && m <= 525600)) return t('downloads.limits.minutesWhole');
	return '';
}

/** Bytes per second as whole KB/s for the speed boxes (0 is no cap), 1000-based like every speed the page shows. */
export const kbOf = (b: number | undefined) => (b && b > 0 ? Math.round(b / 1000) : 0);

/** Reads a KB/s box; blank, negative or not a number is no cap. */
export const bytesOf = (kb: number | string) => {
	const v = Number(kb);
	return Number.isFinite(v) && v > 0 ? Math.round(v * 1000) : 0;
};

const PROBLEM_WORD: Record<TrackerProblem, string> = {
	get error() { return t('downloads.tracker.error'); },
	get warning() { return t('downloads.tracker.warning'); },
	get announce() { return t('downloads.tracker.announce'); }
};

/** The tracker hosts and problems in these rows, with counts. */
export function trackerOptions(rows: Tracked[]): { key: string; label: string; count: number }[] {
	const hosts = new Map<string, number>();
	const probs = new Map<TrackerProblem, number>();
	for (const r of rows) {
		if (r.tracker) hosts.set(r.tracker, (hosts.get(r.tracker) ?? 0) + 1);
		if (r.tracker_problem) probs.set(r.tracker_problem, (probs.get(r.tracker_problem) ?? 0) + 1);
	}
	return [
		...(['error', 'warning', 'announce'] as TrackerProblem[]).filter((p) => probs.has(p)).map((p) => ({ key: `p:${p}`, label: PROBLEM_WORD[p], count: probs.get(p)! })),
		...[...hosts].sort((a, b) => a[0].localeCompare(b[0])).map(([h, n]) => ({ key: `host:${h}`, label: h, count: n }))
	];
}

/** Whether a row passes the tracker filter; 'p:any' is any tracker problem. */
export function byTracker(r: Tracked, key: string): boolean {
	if (!key) return true;
	if (key === 'p:any') return !!r.tracker_problem;
	if (key.startsWith('p:')) return r.tracker_problem === key.slice(2);
	return key.startsWith('host:') && r.tracker === key.slice(5);
}

/** Reads the Add trackers box: one address per line. */
export function splitTrackers(text: string): { urls: string[]; bad: string[] } {
	const urls: string[] = [];
	const bad: string[] = [];
	for (const raw of text.split(/\r?\n/)) {
		const l = raw.trim();
		if (!l) continue;
		if (/^(https?|udp):\/\/[^\s|]+$/i.test(l)) urls.push(l);
		else bad.push(l);
	}
	return { urls, bad };
}

/** Keeps the files whose path holds every typed word (any case). */
export function filterFiles<T extends { name: string }>(files: T[], q: string): T[] {
	const words = q.toLowerCase().split(/\s+/).filter(Boolean);
	if (!words.length) return files;
	return files.filter((f) => {
		const n = f.name.toLowerCase();
		return words.every((w) => n.includes(w));
	});
}

/** Every file index at or under the picked keys (a file, or a folder and its files), once each. */
export function indexesUnder(files: { name: string; index: number }[], keys: Iterable<string>): number[] {
	const out = new Set<number>();
	for (const k of keys) for (const f of files) if (f.name === k || f.name.startsWith(k + '/')) out.add(f.index);
	return [...out].sort((a, b) => a - b);
}

/** The chip beside the status pills; '' when no tracker has a problem. */
export function problemWords(n: number): string {
	return n > 0 ? t(n === 1 ? 'downloads.tracker.issue.one' : 'downloads.tracker.issue.other', { count: n }) : '';
}

/** Picks the rows a toolbar button means and says why it is off: nothing picked, every pick read-only, or none in the right state. `want` is 'paused' for Start, 'running' for Pause, 'any' for Remove. */
export function toolbarTargets<T extends ToolTarget>(picked: T[], open: T | undefined, want: 'paused' | 'running' | 'any', able: (r: T) => boolean = () => true): { rows: T[]; why: string } {
	const pool = picked.length ? picked : open ? [open] : [];
	if (!pool.length) return { rows: [], why: t('downloads.target.select') };
	const live = pool.filter((r) => r.can_act && able(r));
	if (!live.length) return { rows: [], why: pool.every((r) => !r.can_act || r.client_locked) ? t('downloads.target.readOnly') : t('downloads.target.notFor') };
	const rows = want === 'any' ? live : live.filter((r) => (r.state === 'paused') === (want === 'paused'));
	if (!rows.length) return { rows: [], why: want === 'paused' ? t('downloads.target.paused') : t('downloads.target.running') };
	return { rows, why: '' };
}

/** The toolbar word with the pick count when more than one row is meant. */
export const countLabel = (word: string, n: number) => (n > 1 ? `${word} ${n}` : word);
