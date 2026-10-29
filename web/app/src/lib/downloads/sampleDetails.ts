// Sample Files, Peers and Log for an opened download. The server reports none
// of these yet, so the showcase build (VITE_DEMO_DETAILS=1) fills them with
// realistic made-up values built from the row's real name, size, progress,
// speed, seeds, peers and dates. Every other build shows "Not reported yet".
// Pure, so it can be tested.
import type { DownloadRow } from '../downloaders.ts';
import { stageOf } from './status-tabs.ts';
import { size as bytes } from './size.ts';

export const demoDetails = import.meta.env.VITE_DEMO_DETAILS === '1';

export interface SampleFile { name: string; size: number; done: number }
export interface SamplePeer { ip: string; client: string; progress: number; down: number; up: number }
export interface SampleLine { at: string; text: string; bad?: boolean }

/** A small repeatable random run per row, so a row looks the same every time. */
function rng(seed: number) {
	let s = (seed * 2654435761) >>> 0 || 1;
	return () => {
		s ^= s << 13; s >>>= 0;
		s ^= s >>> 17;
		s ^= s << 5; s >>>= 0;
		return s / 4294967296;
	};
}
const clamp = (n: number) => Math.max(0, Math.min(1, n || 0));
const isTorrent = (r: DownloadRow) => (r.protocol ?? '') !== 'usenet';
const base = (r: DownloadRow) => (r.name || r.title || 'Download').replace(/\.(mkv|mp4|avi|nzb|torrent)$/i, '');

export function sampleFiles(r: DownloadRow): SampleFile[] {
	const total = r.size || 1.4 * 2 ** 30;
	const p = clamp(r.progress);
	const n = base(r);
	let list: { name: string; size: number }[];
	if (isTorrent(r)) {
		const eps = r.episodes?.length ? r.episodes : null;
		const nfo = 4_300;
		const srt = 92_000;
		if (eps) {
			const each = Math.floor((total - nfo) / eps.length);
			list = [...eps.map((e) => ({ name: `${n}/${n.replace(/S\d+(E\d+)?.*/i, '')}${e.label}.mkv`, size: each })), { name: `${n}/${n}.nfo`, size: nfo }];
		} else {
			list = [
				{ name: `${n}/${n}.mkv`, size: total - nfo - 2 * srt },
				{ name: `${n}/Subs/English.srt`, size: srt },
				{ name: `${n}/Subs/Spanish.srt`, size: srt },
				{ name: `${n}/${n}.nfo`, size: nfo }
			];
		}
	} else {
		const parts = Math.max(2, Math.min(9, Math.ceil(total / (350 * 2 ** 20))));
		const par = Math.round(total * 0.06);
		const rar = Math.floor((total - par - 2_100) / parts);
		list = [
			...Array.from({ length: parts }, (_, i) => ({ name: `${n}.part${String(i + 1).padStart(2, '0')}.rar`, size: rar })),
			{ name: `${n}.par2`, size: 40_000 },
			{ name: `${n}.vol000+01.par2`, size: Math.round(par / 2) },
			{ name: `${n}.vol001+02.par2`, size: Math.round(par / 2) },
			{ name: `${n}.nfo`, size: 2_100 }
		];
	}
	// Files fill in order, so the first are done and the last are not started.
	let before = 0;
	return list.map((f) => {
		const done = Math.max(0, Math.min(f.size, p * total - before));
		before += f.size;
		return { name: f.name, size: f.size, done };
	});
}

const CLIENTS = ['qBittorrent 5.0.2', 'Transmission 4.0.5', 'Deluge 2.1.1', 'libtorrent 2.0.10', 'µTorrent 3.6.0', 'BiglyBT 3.7.0.0', 'rTorrent 0.9.8'];

export function samplePeers(r: DownloadRow): SamplePeer[] {
	const rand = rng(r.id + 7);
	const count = Math.max(0, Math.min(12, r.peers ?? Math.round(4 + rand() * 6)));
	const speed = r.speed || 0;
	const weights = Array.from({ length: count }, () => 0.2 + rand());
	const sum = weights.reduce((a, b) => a + b, 0) || 1;
	return weights.map((w, i) => ({
		ip: `${[24, 37, 46, 73, 81, 92, 151, 176, 188, 203][Math.floor(rand() * 10)]}.${Math.floor(rand() * 200) + 20}.•.•`,
		client: CLIENTS[Math.floor(rand() * CLIENTS.length)],
		progress: i < (r.seeds ?? 0) ? 1 : clamp(0.1 + rand() * 0.9),
		down: Math.round((speed * w) / sum),
		up: rand() < 0.4 ? Math.round(rand() * 60_000) : 0
	}));
}

/** upRatio is a believable share ratio for the Info tab. */
export function sampleRatio(r: DownloadRow): string {
	const rand = rng(r.id + 3);
	return clamp(r.progress) < 1 ? '0.00' : (0.2 + rand() * 1.6).toFixed(2);
}

const stamp = (t: number) => {
	const d = new Date(t);
	const p = (n: number) => String(n).padStart(2, '0');
	return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
};

export function sampleLog(r: DownloadRow, now = Date.now()): SampleLine[] {
	const rand = rng(r.id + 11);
	const stage = stageOf(r);
	const t0 = r.started_at ? new Date(r.started_at).getTime() : now - 20 * 60_000;
	const end = r.finished_at ? new Date(r.finished_at).getTime() : now;
	const span = Math.max(60_000, end - t0);
	const n = base(r);
	const at = (f: number) => stamp(t0 + span * f);
	const p = clamp(r.progress);
	const out: SampleLine[] = [];
	if (isTorrent(r)) {
		out.push({ at: at(0), text: `Torrent added: ${n}` }, { at: at(0.01), text: 'Metadata received (1 tracker, 3 files)' }, { at: at(0.02), text: `Connected to ${Math.max(3, r.peers ?? 8)} peers` });
		if (p > 0.1) out.push({ at: at(0.1), text: `Piece hash check passed (${Math.round(p * 100)}% of pieces verified)` });
		if (p >= 1 || stage === 'seeding' || stage === 'done') out.push({ at: at(0.92), text: 'Download finished; all pieces verified' }, { at: at(0.93), text: 'Seeding started' });
		if (stage === 'failed') out.push({ at: at(0.8), text: 'Tracker reported: torrent not registered', bad: true }, { at: at(0.81), text: 'No peers have the missing pieces; marked as stalled', bad: true });
	} else {
		const arts = Math.round(((r.size || 1.4 * 2 ** 30) / 716_800) * p);
		out.push({ at: at(0), text: `NZB added: ${n}` }, { at: at(0.02), text: 'Connected to news server (ssl, 20 connections)' });
		if (p > 0) out.push({ at: at(0.5), text: `${arts.toLocaleString('en-US')} articles fetched, ${Math.floor(rand() * 9)} missing` });
		if (p >= 1 || stage === 'done' || stage === 'seeding') {
			out.push({ at: at(0.9), text: 'Par2 verify: all files intact' }, { at: at(0.94), text: 'Unpacking rar set' }, { at: at(0.98), text: `Moved to the completed folder: ${n}` });
		}
		if (stage === 'failed') out.push({ at: at(0.7), text: `Par2 repair failed: 14 blocks missing, only 9 recovery blocks available`, bad: true }, { at: at(0.71), text: 'Download failed: not enough data to repair', bad: true });
	}
	return out;
}

// ---- The live-review additions: sample values for the opened card and the header. ----

const MB = 2 ** 20;
const dur = (s: number) => (s >= 3600 ? `${Math.floor(s / 3600)} h ${Math.floor((s % 3600) / 60)} min` : s >= 60 ? `${Math.floor(s / 60)} min ${s % 60} s` : `${s} s`);

export interface Story { facts: { label: string; value: string }[]; steps: { text: string; bad?: boolean }[] }

/** isProblem: the rows whose opened card tells the full story. */
export const isProblem = (r: DownloadRow) => stageOf(r) === 'failed' || stageOf(r) === 'decide';

/** sampleStory: what went wrong and what was tried, for a problem row. */
export function sampleStory(r: DownloadRow): Story {
	const rand = rng(r.id + 21);
	const size = r.size || 1.4 * 2 ** 30;
	const secs = Math.round(600 + rand() * 2400);
	const avg = Math.round(size * clamp(r.progress || 0.7) / secs);
	if (isTorrent(r)) {
		const missing = Math.round(4 + rand() * 40);
		return {
			facts: [
				{ label: 'Missing', value: `${missing} of ${Math.round(size / (4 * MB)).toLocaleString('en-US')} pieces` },
				{ label: 'Source', value: 'Sample Tracker A (sample.tracker.invalid)' },
				{ label: 'Average speed', value: `${bytes(avg)}/s` },
				{ label: 'Time taken', value: dur(secs) }
			],
			steps: [
				{ text: 'Checked the files again: all pieces that exist are good' },
				{ text: 'Asked the trackers again: 0 seeds have the missing pieces', bad: true },
				{ text: 'Waited 30 min for new peers: none had them', bad: true },
				{ text: 'HolyArrs will look for another copy' }
			]
		};
	}
	const missing = Math.round(8 + rand() * 30);
	const blocks = Math.round(4 + rand() * 8);
	return {
		facts: [
			{ label: 'Missing', value: `${missing} articles of ${Math.round(size / 716_800).toLocaleString('en-US')}` },
			{ label: 'Source', value: 'Sample Index 1 (sample.index.invalid)' },
			{ label: 'Server', value: 'Sample News 1 (priority 1), then Sample News 2' },
			{ label: 'Average speed', value: `${bytes(avg)}/s` },
			{ label: 'Time taken', value: dur(secs) }
		],
		steps: [
			{ text: `Par2 verify: ${missing + 6} blocks damaged or missing` },
			{ text: `Par2 repair: ${blocks} recovery blocks available, ${missing + 6} needed`, bad: true },
			{ text: 'Unpack skipped: the files are not complete', bad: true },
			{ text: 'HolyArrs will look for another copy' }
		]
	};
}

export interface SwarmInfo {
	seedsOn: number; seedsOf: number; peersOn: number; peersOf: number;
	nextCheck: number; private: boolean; wasted: number;
	sources: { name: string; state: 'working' | 'updating' | 'error'; message: string }[];
}

/** sampleSwarm: the torrent depth: connected of swarm, tracker check, sources. */
export function sampleSwarm(r: DownloadRow): SwarmInfo {
	const rand = rng(r.id + 31);
	const seedsOn = r.seeds ?? Math.round(rand() * 6);
	const peersOn = r.peers ?? Math.round(rand() * 14);
	const priv = rand() < 0.4;
	return {
		seedsOn, seedsOf: seedsOn + Math.round(8 + rand() * 90),
		peersOn, peersOf: peersOn + Math.round(5 + rand() * 60),
		nextCheck: Math.round(120 + rand() * 1500),
		private: priv,
		wasted: Math.round(rand() * 24 * MB),
		sources: [
			{ name: 'Sample Tracker A', state: 'working', message: 'Working: announced OK' },
			{ name: 'Sample Tracker B', state: rand() < 0.5 ? 'updating' : 'working', message: 'Updating: announcing now' },
			...(priv ? [] : [{ name: 'Sample Tracker C', state: 'error' as const, message: 'Error: connection timed out' }])
		]
	};
}

const PRIORITY = ['High', 'Normal', 'Normal', 'Low'];
/** fileExtras: each sample file's priority and how much of it the swarm has. */
export function fileExtras(r: DownloadRow, i: number) {
	const rand = rng(r.id * 13 + i);
	return { priority: i === 0 ? 'Normal' : PRIORITY[Math.floor(rand() * 4)], availability: i === 0 ? 1 + Math.round(rand() * 8) / 10 : 1 };
}

/** sampleServers: how much each news server supplied (usenet). */
export function sampleServers(r: DownloadRow): { name: string; bytes: number; missing: number }[] {
	const rand = rng(r.id + 41);
	const got = (r.size || 1.4 * 2 ** 30) * clamp(r.progress);
	const first = Math.round(got * (0.7 + rand() * 0.2));
	return [
		{ name: 'Sample News 1', bytes: first, missing: Math.round(rand() * 20) },
		{ name: 'Sample News 2', bytes: Math.round(got - first), missing: Math.round(rand() * 6) }
	];
}

export const sampleHeader = {
	sessionDown: 18.4 * 2 ** 30, sessionUp: 6.1 * 2 ** 30,
	free: 1.82 * 2 ** 40, today: 12.6 * 2 ** 30, month: 214 * 2 ** 30, all: 3.7 * 2 ** 40
};

export interface TreeRow { key: string; name: string; depth: number; dir: boolean; size: number; done: number; i: number }

/** fileTree turns the flat file list into folder rows and file rows, hiding what sits inside a closed folder. */
export function fileTree(files: SampleFile[], closed: (key: string) => boolean): TreeRow[] {
	const dirs = new Map<string, { size: number; done: number; order: number }>();
	files.forEach((f) => {
		const parts = f.name.split('/');
		for (let d = 1; d < parts.length; d++) {
			const key = parts.slice(0, d).join('/');
			const e = dirs.get(key) ?? { size: 0, done: 0, order: dirs.size };
			e.size += f.size;
			e.done += f.done;
			dirs.set(key, e);
		}
	});
	const out: TreeRow[] = [];
	const seen = new Set<string>();
	files.forEach((f, i) => {
		const parts = f.name.split('/');
		for (let d = 1; d < parts.length; d++) {
			const key = parts.slice(0, d).join('/');
			if (seen.has(key)) continue;
			seen.add(key);
			if (parts.slice(0, d - 1).some((_, k) => closed(parts.slice(0, k + 1).join('/')))) continue;
			const e = dirs.get(key)!;
			out.push({ key, name: parts[d - 1], depth: d - 1, dir: true, size: e.size, done: e.done, i: -1 });
		}
		for (let k = 1; k < parts.length; k++) if (closed(parts.slice(0, k).join('/'))) return;
		out.push({ key: f.name, name: parts[parts.length - 1], depth: parts.length - 1, dir: false, size: f.size, done: f.done, i });
	});
	return out;
}

export interface SampleTracker { source: string; status: string; state: 'working' | 'updating' | 'error'; peers: number; seeds: number; message: string; next: number }

/** sampleTrackers: one row per source, with its status and the countdown to its next check. */
export function sampleTrackers(r: DownloadRow): SampleTracker[] {
	const rand = rng(r.id + 51);
	const sw = sampleSwarm(r);
	return sw.sources.map((x, i) => ({
		source: `${x.name.toLowerCase().replace(/ /g, '-')}.invalid`,
		status: x.state === 'working' ? 'Working' : x.state === 'updating' ? 'Updating' : 'Error',
		state: x.state,
		peers: x.state === 'error' ? 0 : Math.round(sw.peersOf * (0.4 + rand() * 0.6)),
		seeds: x.state === 'error' ? 0 : Math.round(sw.seedsOf * (0.4 + rand() * 0.6)),
		message: x.message.replace(/^[A-Za-z]+: /, ''),
		next: x.state === 'error' ? 0 : Math.max(30, sw.nextCheck - i * 400)
	}));
}
