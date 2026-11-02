// The Downloads column system: which columns exist, their default order,
// visibility and width, and how a person's own layout is kept in this browser
// (versioned, so an older saved shape still loads). Pure, so it can be tested.

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
	| 'ratio'
	| 'queue'
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
	/** Right-aligned numbers. */
	num?: boolean;
	/** Torrent-only: Usenet rows show a dash. */
	torrent?: boolean;
	/** The Downloaders do not send this yet: listed in the picker, but cannot be shown. */
	missing?: boolean;
}

export const COLUMNS: ColDef[] = [
	// Eight on by default, each filled on nearly every row. Time left lives under the Progress bar;
	// the finished date lives in the pane's Dates.
	{ id: 'poster', label: 'Poster', on: true, width: 52 },
	{ id: 'kind', label: 'Type', on: true, width: 84 },
	{ id: 'title', label: 'Title', on: true, width: 310 },
	{ id: 'proto', label: 'Downloader', on: true, width: 60 },
	{ id: 'progress', label: 'Progress', on: true, width: 120 },
	{ id: 'status', label: 'Status', on: true, width: 150 },
	{ id: 'size', label: 'Size', on: true, width: 100, num: true },
	{ id: 'requested', label: 'Requested by', on: true, width: 130 },
	{ id: 'quality', label: 'Quality', on: false, width: 90 },
	{ id: 'added', label: 'Added', on: false, width: 90, num: true },
	// Seeds and peers in one cell: connected (in the swarm) for each.
	{ id: 'swarm', label: 'Swarm', on: false, width: 130, num: true, torrent: true },
	{ id: 'ratio', label: 'Ratio', on: false, width: 60, num: true, torrent: true },
	{ id: 'queue', label: 'Queue', on: false, width: 64, num: true },
	{ id: 'tracker', label: 'Tracker', on: false, width: 140, torrent: true },
	// The Source (indexer) it was found on, the same word as Settings > Sources.
	{ id: 'source', label: 'Source', on: false, width: 120 },
	{ id: 'category', label: 'Category', on: false, width: 100 }
];

export const COL: Record<ColId, ColDef> = Object.fromEntries(COLUMNS.map((c) => [c.id, c])) as Record<ColId, ColDef>;
export const IDS: ColId[] = COLUMNS.map((c) => c.id);
const isId = (v: unknown): v is ColId => typeof v === 'string' && v in COL;

export const MAX_W = 3000;
export const MIN_W = 40;

export interface Layout {
	/** Every column, in the person's order (hidden ones too). */
	order: ColId[];
	hidden: ColId[];
	widths: Record<ColId, number>;
	/** The person set the Title width; until then it gives way when room is short. */
	titleSet?: boolean;
	/** The column the list is sorted by, and which way; none keeps the tab's own order. */
	sort?: Sort;
}

export interface Sort {
	id: ColId;
	desc: boolean;
}

export const KEY = 'holyarrs.downloads.layout';
/** The first shape: widths only, keyed title/bar/st/size/time. */
export const OLD_KEY = 'holyarrs.downloads.columns';
export const VERSION = 2;

export function defaults(): Layout {
	return {
		order: [...IDS],
		hidden: COLUMNS.filter((c) => !c.on).map((c) => c.id),
		widths: Object.fromEntries(COLUMNS.map((c) => [c.id, c.width])) as Record<ColId, number>
	};
}

const width = (v: unknown, d: number) => (typeof v === 'number' && Number.isFinite(v) ? Math.round(Math.max(MIN_W, Math.min(MAX_W, v))) : d);

/** normalize keeps every known column once, drops unknown ones, and appends
 *  columns added since the layout was saved in their default spot. */
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
	// Columns added since the layout was saved start as their default says.
	const saved = Array.isArray(v.order) ? (v.order as unknown[]) : [];
	const hidden = Array.isArray(v.hidden) ? IDS.filter((id) => (v.hidden as unknown[]).includes(id) || (saved.length > 0 && !saved.includes(id) && !COL[id].on)) : d.hidden;
	const w = (v.widths ?? {}) as Record<string, unknown>;
	const widths = Object.fromEntries(IDS.map((id) => [id, width(w[id], d.widths[id])])) as Record<ColId, number>;
	const s = v.sort as Partial<Sort> | undefined;
	const sort = s && isId(s.id) ? { id: s.id, desc: s.desc === true } : undefined;
	return { order, hidden, widths, titleSet: v.titleSet === true, sort };
}

/** load reads the saved layout; with none, it carries over the first shape's widths. */
export function load(raw: string | null, oldRaw: string | null): Layout {
	try {
		const v = raw ? JSON.parse(raw) : null;
		if (v && typeof v === 'object' && v.v === VERSION) return normalize(v);
	} catch {
		/* unreadable: fall through */
	}
	const d = defaults();
	try {
		const o = oldRaw ? JSON.parse(oldRaw) : null;
		if (o && typeof o === 'object') {
			const map: [string, ColId][] = [['title', 'title'], ['bar', 'progress'], ['st', 'status'], ['size', 'size']];
			for (const [from, to] of map) d.widths[to] = width(o[from], d.widths[to]);
		}
	} catch {
		/* nothing usable */
	}
	return d;
}

export function dump(l: Layout): string {
	return JSON.stringify({ v: VERSION, order: l.order, hidden: l.hidden, widths: l.widths, titleSet: !!l.titleSet, sort: l.sort });
}

/** visible is the columns on screen, in order (never one the Downloaders do not send). */
export function visible(l: Layout): ColId[] {
	return l.order.filter((id) => !l.hidden.includes(id) && !COL[id].missing);
}

/** move puts column `id` at index `to` of the full order. */
export function move(order: ColId[], id: ColId, to: number): ColId[] {
	const rest = order.filter((x) => x !== id);
	const at = Math.max(0, Math.min(rest.length, to));
	return [...rest.slice(0, at), id, ...rest.slice(at)];
}

/** moveVisible drops `id` before visible column number `slot` (slot = count: at the end). */
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

/** nextSort is a header click: a new column sorts up (numbers and dates
 *  start high), the same column flips, and a third click clears it. */
export function nextSort(cur: Sort | undefined, id: ColId): Sort | undefined {
	const first = !!COL[id].num;
	if (!cur || cur.id !== id) return { id, desc: first };
	if (cur.desc === first) return { id, desc: !first };
	return undefined;
}

/** sortRows orders rows by one key; empty values (null, undefined, '') go last either way. */
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

/** quality reads a Release name: "2160p HDR", "1080p DV", "720p" ('' when it says nothing). */
export function quality(name: string | undefined): string {
	const n = (name ?? '').toLowerCase();
	if (!n) return '';
	const res = /\b(2160p|4k|uhd)\b/.test(n) ? '2160p' : /\b1080[pi]\b/.test(n) ? '1080p' : /\b720p\b/.test(n) ? '720p' : /\b(480p|576p|sd)\b/.test(n) ? '480p' : '';
	const dv = /\b(dv|dovi|dolby[ ._-]?vision)\b/.test(n);
	const hdr = /\bhdr/.test(n);
	const tag = dv ? 'DV' : hdr ? 'HDR' : '';
	return [res, tag].filter(Boolean).join(' ');
}

/** shortAge says how old a post is: "5 h", "3 d", "2 y" ('' when unknown). */
export function shortAge(iso: string | undefined, now = Date.now()): string {
	const t = iso ? Date.parse(iso) : NaN;
	if (!Number.isFinite(t)) return '';
	const h = Math.max(0, (now - t) / 3600e3);
	if (h < 1) return `${Math.max(1, Math.round(h * 60))} m`;
	if (h < 24) return `${Math.floor(h)} h`;
	const d = h / 24;
	return d < 365 ? `${Math.floor(d)} d` : `${Math.floor(d / 365)} y`;
}
