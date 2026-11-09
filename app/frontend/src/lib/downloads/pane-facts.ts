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
export const PRIOS: { v: number; label: string }[] = [{ v: 0, label: 'Skip' }, { v: 1, label: 'Normal' }, { v: 6, label: 'High' }, { v: 7, label: 'Maximum' }];
export const flagsOf = (p: { progress: number; speed: number; up_speed: number }) => `${p.progress >= 1 ? 'S ' : ''}${p.speed > 0 ? 'D ' : ''}${p.up_speed > 0 ? 'U' : ''}`.trim() || '–';
export const CONN_WORD = { connected: 'Connected', firewalled: 'Firewalled', disconnected: 'Disconnected' } as const;
export const CONN_HINT = { connected: 'Other peers can reach this Downloader.', firewalled: 'Peers can’t connect in. Open the listening port.', disconnected: 'The Downloader has no network.' } as const;
export const TRACKER_WORD: Record<string, string> = { working: 'Working', not_contacted: 'Not contacted yet', not_working: 'Not working', error: 'Error', unreachable: 'Unreachable', disabled: 'Off' };

// Panel reads never show the server's own words, only a calm line and Retry.
export const CALM_ERR = "Couldn't read this from the Downloader just now.";

export function infoGroups(r: DownloadRow, d: Detail | undefined): { cap: string; rows: Fact[] }[] {
	const t = d?.torrent;
	const speed = t?.speed ?? r.speed ?? 0;
	const up = t?.up_speed ?? r.up_speed ?? 0;
	const eta = t?.eta ?? r.eta ?? 0;
	const size = t?.size || r.size || 0;
	const seeding = (t ? t.downloaded >= t.size && t.size > 0 : false) || r.progress >= 1;
	const got = t ? `${bytes(t.downloaded)} of ${bytes(t.size)}` : r.size ? `${bytes((r.size * percent(r.progress)) / 100)} of ${bytes(r.size)}` : '';
	const f = (k: string, v: string | false | undefined | null, tid?: string, path?: boolean): Fact | null => (v ? { k, v, tid, path } : null);
	const ratio = f('Share ratio', t ? t.ratio.toFixed(2) : r.ratio != null ? Number(r.ratio).toFixed(2) : '');
	const uploaded = f('Uploaded', t ? bytes(t.uploaded) : '', 'dl-info-uploaded');
	const seedTime = f('Seeding time', t?.seeding_time ? duration(t.seeding_time) : '', 'dl-info-seedtime');
	const activity = f('Last activity', t?.last_activity ? ago(t.last_activity) : '', 'dl-info-activity');
	const downRow = f('Down', speed ? `${bytes(speed)}/s` : '');
	const upRow = f('Up', up ? `${bytes(up)}/s` : '');
	const transfer = seeding
		? [ratio, uploaded, seedTime, activity, f('Size', size ? bytes(size) : ''), upRow, downRow, f('Wasted', t?.wasted ? bytes(t.wasted) : '')]
		: [f('Downloaded', got), downRow, upRow, f('Time left', eta ? duration(eta) : ''), uploaded, ratio, activity, f('Wasted', t?.wasted ? bytes(t.wasted) : '')];
	const swarm = [f('Seeds', t ? swarmLine(t.seeds, t.seeds_total) : ''), f('Peers', t ? swarmLine(t.peers, t.peers_total) : ''), f('Private', t?.private == null ? '' : t.private ? 'Yes' : 'No')];
	const dates = [
		f('Added', dateTime(t?.added_at ?? r.started_at)),
		// Usenet: how old the post is on the newsgroups (retention), a pane fact rather than a column.
		f('Posted', r.posted_at ? `${shortAge(r.posted_at)} ago` : ''),
		f('Done', dateTime(t?.done_at ?? r.finished_at)),
		f('Time active', t?.time_active ? duration(t.time_active) : ''),
		seeding ? null : seedTime,
		// The row shows a show's episodes instead of the release name, and a tooltip is out of reach on touch.
		f('Release', r.name, 'dl-info-release', true),
		f('Save path', t?.save_path, undefined, true),
		f('Downloader', d?.downloader || r.downloader),
		f('Requested by', d?.requested_by, 'dl-info-asker')
	];
	const caps = r.limit_down || r.limit_up ? `${r.limit_down ? `${bytes(r.limit_down)}/s down` : 'no cap down'} · ${r.limit_up ? `${bytes(r.limit_up)}/s up` : 'no cap up'}` : '';
	const rules = [f('Speed limits', caps, 'dl-info-caps'), f('Seeding rules', r.limits ? limitsWords(r.limits) : '', 'dl-info-limits')];
	const keep = (xs: (Fact | null)[]) => xs.filter((x): x is Fact => !!x);
	return [
		{ cap: 'Transfer', rows: keep(transfer) },
		{ cap: 'Swarm', rows: keep(swarm) },
		{ cap: 'Dates', rows: keep(dates) },
		{ cap: 'Rules', rows: keep(rules) }
	].filter((g) => g.rows.length);
}

// A Usenet job's Info in the same shape as a torrent's: what moved, when and where it sits.
export function usenetGroups(r: DownloadRow, d: Detail | undefined, im: ImportRecord | undefined): { cap: string; rows: Fact[] }[] {
	const f = (k: string, v: string | false | undefined | null, tid?: string, path?: boolean): Fact | null => (v ? { k, v, tid, path } : null);
	const done = r.progress >= 1;
	const got = r.size ? (done ? bytes(r.size) : `${bytes((r.size * percent(r.progress)) / 100)} of ${bytes(r.size)}`) : '';
	const transfer = [f(done ? 'Size' : 'Downloaded', got), f('Down', r.speed ? `${bytes(r.speed)}/s` : ''), f('Time left', !done && r.eta ? duration(r.eta) : '')];
	const dates = [f('Posted', r.posted_at ? `${shortAge(r.posted_at)} ago` : ''), f('Added', dateTime(r.started_at)), f('Done', dateTime(r.finished_at))];
	const folder = im?.Plan?.find((p) => p.target_path)?.target_path || r.path;
	const where = [f('Release', r.name, 'dl-info-release', true), f('Downloader', d?.downloader || r.downloader), f('Category', r.category), f('Folder', folder, undefined, true), f('Requested by', d?.requested_by || r.requested_by, 'dl-info-asker')];
	const keep = (xs: (Fact | null)[]) => xs.filter((x): x is Fact => !!x);
	return [
		{ cap: 'Transfer', rows: keep(transfer) },
		{ cap: 'Dates', rows: keep(dates) },
		{ cap: 'Where', rows: keep(where) }
	].filter((g) => g.rows.length);
}

export const fileIndexes = (files: TFile[], key: string) => files.filter((f) => f.name === key || f.name.startsWith(key + '/')).map((f) => f.index);
