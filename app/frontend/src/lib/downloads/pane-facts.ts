import { t as say } from '#lib/core/i18n/i18n.svelte.ts';
import { size as bytes, protocolOf } from './size.ts';
import { swarmLine } from './panel-words.ts';
import { shortAge } from './columns.ts';
import { limitsWords } from './manage.ts';
import type { ImportRecord } from './types.ts';
import type { UsenetView } from './usenet.ts';
import { ago, dateTime, duration, percent } from '../core/format.ts';
import type { DownloadRow } from '../core/types.ts';

export type PaneTab = (typeof TORRENT_TABS)[number] | (typeof USENET_TABS)[number];

export type TorrentDetail = { size: number; downloaded: number; uploaded: number; wasted: number; speed: number; up_speed: number; eta: number; ratio: number; seeds: number; seeds_total: number; peers: number; peers_total: number; added_at?: string; done_at?: string; save_path: string; private?: boolean; seeding_time?: number; time_active?: number; last_seen?: string; last_activity?: string };
export type Detail = { downloader: string; requested_by: string; torrent?: TorrentDetail; torrent_error?: string; log: { at: string; kind: string; summary: string }[] };
export type TFile = { index: number; name: string; size: number; progress: number; priority: number };
export type Tracker = { url: string; tier: number; status: string; message: string; seeds: number; peers: number; next_check: number; fixed?: boolean; ref?: string };
export type Peer = { address: string; client: string; progress: number; speed: number; up_speed: number; country: string };
export type Det = { id: number; detail?: Detail; files?: TFile[]; filesErr?: string; trackers?: Tracker[]; peers?: Peer[]; swarmErr?: string; usenet?: UsenetView; usenetErr?: string };
export type Fact = { k: string; v: string; tid?: string; path?: boolean };

// The detail tabs follow the download's kind: a torrent has peers and trackers, a Usenet job its servers. Usenet steps sit at the top of its Log.
export const TORRENT_TABS = ['Info', 'Files', 'Peers', 'Trackers', 'Log'] as const;

export const USENET_TABS = ['Info', 'Servers', 'Log'] as const;
export const tabsOf = (r: DownloadRow): readonly PaneTab[] => (protocolOf(r) === 'torrent' ? TORRENT_TABS : USENET_TABS);

export const mmss = (n: number) => `${Math.floor(n / 60)}:${String(n % 60).padStart(2, '0')}`;
export const PRIOS: { v: number; label: string }[] = [{ v: 0, get label() { return say('downloads.prio.skip'); } }, { v: 1, get label() { return say('downloads.prio.normal'); } }, { v: 6, get label() { return say('downloads.prio.high'); } }, { v: 7, get label() { return say('downloads.prio.max'); } }];

/** The words for the detail tabs; the tab ids stay as they are. */
export const TAB_WORD: Record<PaneTab, string> = {
	get Info() { return say('downloads.tab.info'); },
	get Files() { return say('downloads.tab.files'); },
	get Peers() { return say('downloads.tab.peers'); },
	get Trackers() { return say('downloads.tab.trackers'); },
	get Log() { return say('downloads.tab.log'); },
	get Servers() { return say('downloads.tab.servers'); }
};
export const flagsOf = (p: { progress: number; speed: number; up_speed: number }) => `${p.progress >= 1 ? 'S ' : ''}${p.speed > 0 ? 'D ' : ''}${p.up_speed > 0 ? 'U' : ''}`.trim() || '–';
export const CONN_WORD = { get connected() { return say('downloads.conn.connected'); }, get firewalled() { return say('downloads.conn.firewalled'); }, get disconnected() { return say('downloads.conn.disconnected'); } };
export const CONN_HINT = { get connected() { return say('downloads.conn.connectedHint'); }, get firewalled() { return say('downloads.conn.firewalledHint'); }, get disconnected() { return say('downloads.conn.disconnectedHint'); } };
export const TRACKER_WORD: Record<string, string> = {
	get working() { return say('downloads.trk.working'); },
	get not_contacted() { return say('downloads.trk.notContacted'); },
	get not_working() { return say('downloads.trk.notWorking'); },
	get error() { return say('downloads.trk.error'); },
	get unreachable() { return say('downloads.trk.unreachable'); },
	get disabled() { return say('downloads.trk.off'); }
};

// Panel reads never show the server's own words, only a calm line and Retry.
export const calmErr = () => say('downloads.pf.calm');

export function infoGroups(r: DownloadRow, d: Detail | undefined): { cap: string; rows: Fact[] }[] {
	const t = d?.torrent;
	const speed = t?.speed ?? r.speed ?? 0;
	const up = t?.up_speed ?? r.up_speed ?? 0;
	const eta = t?.eta ?? r.eta ?? 0;
	const size = t?.size || r.size || 0;
	const seeding = (t ? t.downloaded >= t.size && t.size > 0 : false) || r.progress >= 1;
	const got = t ? say('downloads.pf.of', { a: bytes(t.downloaded), b: bytes(t.size) }) : r.size ? say('downloads.pf.of', { a: bytes((r.size * percent(r.progress)) / 100), b: bytes(r.size) }) : '';
	const f = (k: string, v: string | false | undefined | null, tid?: string, path?: boolean): Fact | null => (v ? { k, v, tid, path } : null);
	const ratio = f(say('downloads.pf.shareRatio'), t ? t.ratio.toFixed(2) : r.ratio != null ? Number(r.ratio).toFixed(2) : '');
	const uploaded = f(say('downloads.pf.uploaded'), t ? bytes(t.uploaded) : '', 'dl-info-uploaded');
	const seedTime = f(say('downloads.pf.seedingTime'), t?.seeding_time ? duration(t.seeding_time) : '', 'dl-info-seedtime');
	const activity = f(say('downloads.pf.lastActive'), t?.last_activity ? ago(t.last_activity) : '', 'dl-info-activity');
	const downRow = f(say('downloads.pf.down'), speed ? `${bytes(speed)}/s` : '');
	const upRow = f(say('downloads.pf.up'), up ? `${bytes(up)}/s` : '');
	const transfer = seeding
		? [ratio, uploaded, seedTime, activity, f(say('downloads.pf.size'), size ? bytes(size) : ''), upRow, downRow, f(say('downloads.pf.wasted'), t?.wasted ? bytes(t.wasted) : '')]
		: [f(say('downloads.pf.downloaded'), got), downRow, upRow, f(say('downloads.pf.timeLeft'), eta ? duration(eta) : ''), uploaded, ratio, activity, f(say('downloads.pf.wasted'), t?.wasted ? bytes(t.wasted) : '')];
	const swarm = [f(say('downloads.facet.seeds'), t ? swarmLine(t.seeds, t.seeds_total) : ''), f(say('downloads.facet.peers'), t ? swarmLine(t.peers, t.peers_total) : ''), f(say('downloads.pf.private'), t?.private == null ? '' : t.private ? say('health.sections.yes') : say('health.sections.no'))];
	const dates = [
		f(say('downloads.pf.added'), dateTime(t?.added_at ?? r.started_at)),
		// Usenet: how old the post is on the newsgroups (retention), a pane fact rather than a column.
		f(say('downloads.pf.posted'), r.posted_at ? say('downloads.pf.ago', { age: shortAge(r.posted_at) }) : ''),
		f(say('downloads.pf.done'), dateTime(t?.done_at ?? r.finished_at)),
		f(say('downloads.pf.timeActive'), t?.time_active ? duration(t.time_active) : ''),
		seeding ? null : seedTime,
		// The row shows a show's episodes instead of the release name, and a tooltip is out of reach on touch.
		f(say('downloads.pf.release'), r.name, 'dl-info-release', true),
		f(say('downloads.pf.savePath'), t?.save_path, undefined, true),
		f(say('downloads.col.proto'), d?.downloader || r.downloader),
		f(say('downloads.col.requested'), d?.requested_by, 'dl-info-asker')
	];
	const caps = r.limit_down || r.limit_up ? `${r.limit_down ? say('downloads.pf.capDown', { speed: `${bytes(r.limit_down)}/s` }) : say('downloads.pf.noCapDown')} · ${r.limit_up ? say('downloads.pf.capUp', { speed: `${bytes(r.limit_up)}/s` }) : say('downloads.pf.noCapUp')}` : '';
	const rules = [f(say('downloads.steer.speed'), caps, 'dl-info-caps'), f(say('downloads.steer.seed'), r.limits ? limitsWords(r.limits) : '', 'dl-info-limits')];
	const keep = (xs: (Fact | null)[]) => xs.filter((x): x is Fact => !!x);
	return [
		{ cap: say('downloads.pf.capTransfer'), rows: keep(transfer) },
		{ cap: say('downloads.pf.capSwarm'), rows: keep(swarm) },
		{ cap: say('downloads.pf.capDates'), rows: keep(dates) },
		{ cap: say('downloads.pf.capRules'), rows: keep(rules) }
	].filter((g) => g.rows.length);
}

// A Usenet job's Info in the same shape as a torrent's: what moved, when and where it sits.
export function usenetGroups(r: DownloadRow, d: Detail | undefined, im: ImportRecord | undefined): { cap: string; rows: Fact[] }[] {
	const f = (k: string, v: string | false | undefined | null, tid?: string, path?: boolean): Fact | null => (v ? { k, v, tid, path } : null);
	const done = r.progress >= 1;
	const got = r.size ? (done ? bytes(r.size) : say('downloads.pf.of', { a: bytes((r.size * percent(r.progress)) / 100), b: bytes(r.size) })) : '';
	const transfer = [f(done ? say('downloads.pf.size') : say('downloads.pf.downloaded'), got), f(say('downloads.pf.down'), r.speed ? `${bytes(r.speed)}/s` : ''), f(say('downloads.pf.timeLeft'), !done && r.eta ? duration(r.eta) : '')];
	const dates = [f(say('downloads.pf.posted'), r.posted_at ? say('downloads.pf.ago', { age: shortAge(r.posted_at) }) : ''), f(say('downloads.pf.added'), dateTime(r.started_at)), f(say('downloads.pf.done'), dateTime(r.finished_at))];
	const folder = im?.Plan?.find((p) => p.target_path)?.target_path || r.path;
	const where = [f(say('downloads.pf.release'), r.name, 'dl-info-release', true), f(say('downloads.col.proto'), d?.downloader || r.downloader), f(say('downloads.col.category'), r.category), f(say('downloads.pf.folder'), folder, undefined, true), f(say('downloads.col.requested'), d?.requested_by || r.requested_by, 'dl-info-asker')];
	const keep = (xs: (Fact | null)[]) => xs.filter((x): x is Fact => !!x);
	return [
		{ cap: say('downloads.pf.capTransfer'), rows: keep(transfer) },
		{ cap: say('downloads.pf.capDates'), rows: keep(dates) },
		{ cap: say('downloads.pf.capWhere'), rows: keep(where) }
	].filter((g) => g.rows.length);
}

export const fileIndexes = (files: TFile[], key: string) => files.filter((f) => f.name === key || f.name.startsWith(key + '/')).map((f) => f.index);
