// The Downloads column system: which columns exist, their default order, visibility and width, and how a person's layout is kept in this browser. Versioned so an older saved shape still loads. Pure, so it can be tested.
import { t } from '#lib/core/i18n/i18n.svelte.ts';

export type ColId =
	| 'poster'
	| 'kind'
	| 'title'
	| 'proto'
	| 'progress'
	| 'status'
	| 'size'
	| 'quality'
	| 'added'
	| 'swarm'
	| 'tracker'
	| 'requested'
	| 'source'
	| 'category';

export interface ColDef {
	id: ColId;
	label: string;
	/** Shown on a first visit and after Reset. */
	on: boolean;
	width: number;
	num?: boolean;
	/** Torrent-only; Usenet rows show a dash. */
	torrent?: boolean;
	/** Not sent by the Downloaders yet: listed in the picker but cannot be shown. */
	missing?: boolean;
	/** One width, no resize grip. */
	fixed?: boolean;
}

export interface Layout {
	/** Every column in the person's order, hidden ones included. */
	order: ColId[];
	hidden: ColId[];
	widths: Record<ColId, number>;
	/** Set once the person sizes the Title; until then it gives way when room is short. */
	titleSet?: boolean;
	/** The sorted column and direction; none keeps the tab's own order. */
	sort?: Sort;
}

/** Which of the Swarm column's three numbers it sorts by. */
export type SwarmFacet = 'seeds' | 'peers' | 'ratio';

export interface Sort {
	id: ColId;
	desc: boolean;
	/** Swarm only. */
	facet?: SwarmFacet;
}

// Each default width is the widest usual content plus 14px, measured in the app font (12px cells, 11px second lines and headers), so a fresh table reads as auto-fitted. Poster, Type and Downloader snap to their own sizes.
export const COLUMNS: ColDef[] = [
	// Seven columns are on by default, each filled on nearly every row. Time left sits under the Progress bar and the finished date lives in the pane's Dates.
	{ id: 'poster', get label() { return t('downloads.col.poster'); }, on: true, width: 32 },
	// Off by default: the poster and the title's second line already say movie, show or music.
	{ id: 'kind', get label() { return t('downloads.col.kind'); }, on: false, width: 24 },
	{ id: 'title', get label() { return t('downloads.col.title'); }, on: true, width: 310 },
	{ id: 'proto', get label() { return t('downloads.col.proto'); }, on: true, width: 24 },
	{ id: 'progress', get label() { return t('downloads.col.progress'); }, on: true, width: 130 },
	{ id: 'status', get label() { return t('downloads.col.status'); }, on: true, width: 120 },
	{ id: 'size', get label() { return t('downloads.col.size'); }, on: true, width: 60, num: true },
	{ id: 'requested', get label() { return t('downloads.col.requested'); }, on: true, width: 112 },
	{ id: 'quality', get label() { return t('downloads.col.quality'); }, on: false, width: 80 },
	{ id: 'added', get label() { return t('downloads.col.added'); }, on: false, width: 76, num: true },
	// Seeds, peers and ratio on one line; the header sorts by any of the three.
	{ id: 'swarm', get label() { return t('downloads.col.swarm'); }, on: false, width: 132, torrent: true, fixed: true },
	{ id: 'tracker', get label() { return t('downloads.col.tracker'); }, on: false, width: 150, torrent: true },
	// The indexer it was found on, the same word as Settings > Sources.
	{ id: 'source', get label() { return t('downloads.col.source'); }, on: false, width: 96 },
	{ id: 'category', get label() { return t('downloads.col.category'); }, on: false, width: 96 }
];

export const COL: Record<ColId, ColDef> = Object.fromEntries(COLUMNS.map((c) => [c.id, c])) as Record<ColId, ColDef>;
export const IDS: ColId[] = COLUMNS.map((c) => c.id);
const isId = (v: unknown): v is ColId => typeof v === 'string' && v in COL;

export const MAX_W = 3000;
export const MIN_W = 40;

export const KEY = 'holyarrs.downloads.layout';

/** The first saved shape: widths only. */
export const OLD_KEY = 'holyarrs.downloads.columns';

export const VERSION = 3;

/** Widths version 2 gave by default; a saved width still at one of them takes the new default. */
const V2_WIDTHS: Partial<Record<ColId, number>> = { poster: 52, kind: 84, proto: 60, progress: 120, status: 150, size: 100, requested: 130, quality: 90, added: 90, tracker: 140, source: 120, category: 100 };

export function defaults(): Layout {
	return {
		order: [...IDS],
		hidden: COLUMNS.filter((c) => !c.on).map((c) => c.id),
		widths: Object.fromEntries(COLUMNS.map((c) => [c.id, c.width])) as Record<ColId, number>
	};
}

const width = (v: unknown, d: number) => (typeof v === 'number' && Number.isFinite(v) ? Math.round(Math.max(MIN_W, Math.min(MAX_W, v))) : d);

/** Keeps every known column once, drops unknown ones, and adds columns introduced since the save in their default spot. */
export function normalize(v: Partial<Layout> | null | undefined): Layout {
	const d = defaults();
	if (!v || typeof v !== 'object') return d;
	const order: ColId[] = [];
	for (const id of Array.isArray(v.order) ? v.order : []) if (isId(id) && !order.includes(id)) order.push(id);
	for (const id of IDS) {
		if (order.includes(id)) continue;
		const at = IDS.indexOf(id);
		const after = IDS.slice(0, at).reverse().find((p) => order.includes(p));
		order.splice(after ? order.indexOf(after) + 1 : 0, 0, id);
	}
	const saved = Array.isArray(v.order) ? (v.order as unknown[]) : [];
	const hidden = Array.isArray(v.hidden) ? IDS.filter((id) => (v.hidden as unknown[]).includes(id) || (saved.length > 0 && !saved.includes(id) && !COL[id].on)) : d.hidden;
	const w = (v.widths ?? {}) as Record<string, unknown>;
	const widths = Object.fromEntries(IDS.map((id) => [id, width(w[id], d.widths[id])])) as Record<ColId, number>;
	const s = v.sort as Partial<Sort> | undefined;
	const facet = s && s.id === 'swarm' && ['seeds', 'peers', 'ratio'].includes(String(s.facet)) ? (s.facet as SwarmFacet) : undefined;
	const sort = s && isId(s.id) ? { id: s.id, desc: s.desc === true, ...(facet ? { facet } : {}) } : undefined;
	return { order, hidden, widths, titleSet: v.titleSet === true, sort };
}

/** Reads the saved layout; with none, carries over the first shape's widths. */
export function load(raw: string | null, oldRaw: string | null): Layout {
	try {
		const v = raw ? JSON.parse(raw) : null;
		if (v && typeof v === 'object' && v.v === VERSION) return normalize(v);
		// A version 2 layout keeps its order, visibility and sort; widths left at version 2 defaults take the tighter ones.
		if (v && typeof v === 'object' && v.v === 2) {
			const w = { ...((v.widths ?? {}) as Record<string, unknown>) };
			for (const [id, d] of Object.entries(V2_WIDTHS)) if (w[id] === d) delete w[id];
			return normalize({ ...v, widths: w as Record<ColId, number> });
		}
	} catch {
		/* Unreadable: fall through. */
	}
	const d = defaults();
	try {
		const o = oldRaw ? JSON.parse(oldRaw) : null;
		if (o && typeof o === 'object') {
			const map: [string, ColId][] = [['title', 'title'], ['bar', 'progress'], ['st', 'status'], ['size', 'size']];
			for (const [from, to] of map) d.widths[to] = width(o[from], d.widths[to]);
		}
	} catch {
		/* Nothing usable. */
	}
	return d;
}

export function dump(l: Layout): string {
	return JSON.stringify({ v: VERSION, order: l.order, hidden: l.hidden, widths: l.widths, titleSet: !!l.titleSet, sort: l.sort });
}

/** The columns on screen in order, never one the Downloaders do not send. */
export function visible(l: Layout): ColId[] {
	return l.order.filter((id) => !l.hidden.includes(id) && !COL[id].missing);
}

/** Puts column `id` at index `to` of the full order. */
export function move(order: ColId[], id: ColId, to: number): ColId[] {
	const rest = order.filter((x) => x !== id);
	const at = Math.max(0, Math.min(rest.length, to));
	return [...rest.slice(0, at), id, ...rest.slice(at)];
}

/** Drops `id` before visible column number `slot`; slot equal to the count means the end. */
export function moveVisible(l: Layout, id: ColId, slot: number): ColId[] {
	const vis = visible(l).filter((x) => x !== id);
	if (slot >= vis.length) {
		const last = vis[vis.length - 1];
		return last ? move(l.order, id, l.order.filter((x) => x !== id).indexOf(last) + 1) : l.order;
	}
	return move(l.order, id, l.order.filter((x) => x !== id).indexOf(vis[Math.max(0, slot)]));
}

export function toggle(l: Layout, id: ColId): ColId[] {
	return l.hidden.includes(id) ? l.hidden.filter((x) => x !== id) : [...l.hidden, id];
}

/** A header click: a new column sorts up (numbers and dates start high), the same column flips, a third click clears. */
export function nextSort(cur: Sort | undefined, id: ColId, facet?: SwarmFacet): Sort | undefined {
	// Swarm sorts by one of three facets; a new facet starts high, the same facet flips, then clears.
	if (id === 'swarm') {
		const f = facet ?? cur?.facet ?? 'seeds';
		if (!cur || cur.id !== id || cur.facet !== f) return { id, desc: true, facet: f };
		if (cur.desc) return { id, desc: false, facet: f };
		return undefined;
	}
	const first = !!COL[id].num;
	if (!cur || cur.id !== id) return { id, desc: first };
	if (cur.desc === first) return { id, desc: !first };
	return undefined;
}

/** Orders rows by one key; empty values go last either way. */
export function sortRows<T>(rows: T[], key: (r: T) => number | string | null | undefined, desc: boolean): T[] {
	const empty = (v: unknown) => v == null || v === '' || (typeof v === 'number' && !Number.isFinite(v));
	return rows
		.map((r, i) => ({ r, i, k: key(r) }))
		.sort((a, b) => {
			if (empty(a.k) || empty(b.k)) return empty(a.k) === empty(b.k) ? a.i - b.i : empty(a.k) ? 1 : -1;
			const c = typeof a.k === 'number' && typeof b.k === 'number' ? a.k - b.k : String(a.k).localeCompare(String(b.k), undefined, { numeric: true, sensitivity: 'base' });
			return (desc ? -c : c) || a.i - b.i;
		})
		.map((x) => x.r);
}

/** Reads a Release name: "2160p HDR", "1080p DV", "720p", or '' when it says nothing. */
export function quality(name: string | undefined): string {
	const n = (name ?? '').toLowerCase();
	if (!n) return '';
	const res = /\b(2160p|4k|uhd)\b/.test(n) ? '2160p' : /\b1080[pi]\b/.test(n) ? '1080p' : /\b720p\b/.test(n) ? '720p' : /\b(480p|576p|sd)\b/.test(n) ? '480p' : '';
	const dv = /\b(dv|dovi|dolby[ ._-]?vision)\b/.test(n);
	const hdr = /\bhdr/.test(n);
	const tag = dv ? 'DV' : hdr ? 'HDR' : '';
	return [res, tag].filter(Boolean).join(' ');
}

/** Keeps a seed or peer count to four characters: 9999, then "10k". */
export function swarmNum(n: number | undefined | null): string {
	const v = Math.max(0, n ?? 0);
	return v >= 10000 ? `${Math.round(v / 1000)}k` : String(v);
}

/** Keeps a ratio to four characters: 0.51, 12.3, 120. */
export function ratioText(r: number): string {
	return r >= 100 ? r.toFixed(0) : r >= 10 ? r.toFixed(1) : r.toFixed(2);
}

/** How old a post is: "5 h", "3 d", "2 y", or '' when unknown. */
export function shortAge(iso: string | undefined, now = Date.now()): string {
	const at = iso ? Date.parse(iso) : NaN;
	if (!Number.isFinite(at)) return '';
	const h = Math.max(0, (now - at) / 3600e3);
	if (h < 1) return t('downloads.age.m', { n: Math.max(1, Math.round(h * 60)) });
	if (h < 24) return t('downloads.age.h', { n: Math.floor(h) });
	const d = h / 24;
	return d < 365 ? t('downloads.age.d', { n: Math.floor(d) }) : t('downloads.age.y', { n: Math.floor(d / 365) });
}
