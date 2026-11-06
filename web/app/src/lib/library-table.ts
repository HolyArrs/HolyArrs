// The Library's banner table: which columns each library offers, how they
// sort, the member's saved column layout, the Filter menu's counts and the
// saved filters. Kept apart from the page so it can be tested.
import type { TitleView } from './types.ts';
import { matches, type Filter } from './library.ts';
import { libraryState } from './library-state.ts';

export type LibKind = 'movie' | 'show' | 'artist';

export type ColumnId =
	| 'status' | 'network' | 'rules' | 'next' | 'last' | 'seasons' | 'episodes' | 'added' | 'year' | 'size' | 'genres' | 'rating' | 'path'
	| 'release' | 'runtime' | 'series' | 'albums' | 'tracks' | 'latest' | 'requested' | 'quality' | 'kept' | 'showstate' | 'activity' | 'codec' | 'audio';

export interface Column {
	id: ColumnId;
	label: string;
	/** Width in the grid template, e.g. "120px" or "minmax(140px, 1fr)". */
	width: string;
	/** Numbers and dates line up on the right. */
	numeric?: boolean;
	on: boolean;
}

const col = (id: ColumnId, label: string, width: string, on: boolean, numeric = false): Column => ({ id, label, width, on, numeric });

/**
 * columnsFor is each library's columns in their starting order, with the
 * ones shown at first switched on (releases/library-columns-20261021).
 * Status always comes first after the banner.
 */
export function columnsFor(kind: LibKind): Column[] {
	if (kind === 'show')
		return [
			col('status', 'Status', '200px', true),
			col('episodes', 'Episodes', '150px', true),
			col('next', 'Next episode', '118px', true, true),
			col('rules', 'Quality', '120px', true),
			col('size', 'Size', '84px', true, true),
			col('added', 'Added', '104px', true, true),
			col('network', 'Network', '120px', false),
			col('seasons', 'Seasons', '76px', false, true),
			col('last', 'Last aired', '110px', false, true),
			col('year', 'Year', '64px', false, true),
			col('genres', 'Genres', '160px', false),
			col('rating', 'Age rating', '88px', false),
			col('requested', 'Requested by', '130px', false),
			col('quality', 'File quality', '130px', false),
			col('kept', 'Kept up to date', '72px', false),
			col('showstate', 'Show state', '100px', false),
			col('activity', 'Last activity', '110px', false, true),
			col('path', 'Folder', 'minmax(160px, 1fr)', false)
		];
	if (kind === 'artist')
		return [
			col('status', 'Status', '200px', true),
			col('tracks', 'Tracks', '150px', true),
			col('latest', 'Next release', '118px', true, true),
			col('size', 'Size', '84px', true, true),
			col('added', 'Added', '104px', true, true),
			col('rules', 'Quality', '120px', true),
			col('albums', 'Albums', '84px', false, true),
			col('requested', 'Requested by', '130px', false),
			col('kept', 'Kept up to date', '72px', false),
			col('activity', 'Last activity', '110px', false, true),
			col('path', 'Folder', 'minmax(160px, 1fr)', false)
		];
	return [
		col('status', 'Status', '200px', true),
		col('release', 'Out', '176px', true),
		col('rules', 'Quality', '120px', true),
		col('size', 'Size', '84px', true, true),
		col('added', 'Added', '104px', true, true),
		col('year', 'Year', '64px', true, true),
		col('runtime', 'Runtime', '84px', false, true),
		col('rating', 'Age rating', '88px', false),
		col('series', 'Movie series', '160px', false),
		col('genres', 'Genres', '160px', false),
		col('requested', 'Requested by', '130px', false),
		col('quality', 'File quality', '130px', false),
		col('codec', 'Codec', '84px', false),
		col('audio', 'Audio', '150px', false),
		col('kept', 'Kept up to date', '72px', false),
		col('activity', 'Last activity', '110px', false, true),
		col('path', 'Folder', 'minmax(160px, 1fr)', false)
	];
}

/** Saved layout: the columns in order, each on or off. */
export type ColumnLayout = { id: ColumnId; on: boolean }[];

export function columnsKey(member: number | undefined, kind: LibKind): string {
	return `holyarrs.library-columns.${member ?? 'me'}.${kind}`;
}

/**
 * applyLayout puts a saved layout over a library's columns: saved order and
 * on/off first; a column added since goes in at its starting place (so
 * Status lands first), with its starting choice. Unknown ids are dropped.
 */
export function applyLayout(kind: LibKind, saved: ColumnLayout | null | undefined): Column[] {
	const base = columnsFor(kind);
	if (!Array.isArray(saved)) return base;
	const byId = new Map(base.map((c) => [c.id, c]));
	const out: Column[] = [];
	for (const s of saved) {
		const c = s && byId.get(s.id);
		if (!c) continue;
		out.push({ ...c, on: !!s.on });
		byId.delete(s.id);
	}
	base.forEach((c, i) => {
		if (byId.has(c.id)) out.splice(Math.min(i, out.length), 0, c);
	});
	return out;
}

export function loadColumns(member: number | undefined, kind: LibKind, store: Pick<Storage, 'getItem'> | undefined = globalThis.localStorage): Column[] {
	try {
		return applyLayout(kind, JSON.parse(store?.getItem(columnsKey(member, kind)) ?? 'null'));
	} catch {
		return columnsFor(kind);
	}
}

export function saveColumns(member: number | undefined, kind: LibKind, cols: Column[], store: Pick<Storage, 'setItem'> | undefined = globalThis.localStorage) {
	try {
		store?.setItem(columnsKey(member, kind), JSON.stringify(cols.map((c) => ({ id: c.id, on: c.on }))));
	} catch {
		// Private windows may refuse; the starting layout still works.
	}
}

/** moveColumn moves one column from one place to another (drag or the arrow keys). */
export function moveColumn(cols: Column[], from: number, to: number): Column[] {
	if (from === to || from < 0 || from >= cols.length) return cols;
	const next = [...cols];
	const [c] = next.splice(from, 1);
	next.splice(Math.max(0, Math.min(next.length, to)), 0, c);
	return next;
}

// ── Sorting ─────────────────────────────────────────────────────────────

export type SortKey = 'status' | 'name' | 'year' | 'added' | 'size' | 'next' | 'last' | 'progress' | 'release' | 'runtime' | 'network' | 'rules';
export type SortDir = 'asc' | 'desc';

export const sortLabels: Record<SortKey, string> = {
	status: 'Status', name: 'Name', year: 'Year', added: 'Added', size: 'Size', next: 'Next episode', last: 'Last aired',
	progress: 'Progress', release: 'Out date', runtime: 'Runtime', network: 'Network', rules: 'Quality'
};

/** sortsFor lists the sorts a library offers. */
export function sortsFor(kind: LibKind | 'all'): SortKey[] {
	if (kind === 'show') return ['status', 'name', 'next', 'last', 'progress', 'added', 'year', 'size', 'network', 'rules'];
	if (kind === 'artist') return ['status', 'name', 'progress', 'added', 'size', 'next'];
	if (kind === 'movie') return ['status', 'name', 'year', 'release', 'added', 'size', 'runtime', 'rules'];
	return ['name', 'year', 'added', 'size'];
}

/** The column each sort belongs to, so a column header can sort. */
export const columnSort: Partial<Record<ColumnId, SortKey>> = {
	status: 'status',
	year: 'year', added: 'added', size: 'size', next: 'next', last: 'last', episodes: 'progress', tracks: 'progress',
	release: 'release', runtime: 'runtime', network: 'network', rules: 'rules', latest: 'next'
};

/** sortName drops a leading "The", "A" or "An" so "The Office" files under O. */
export function sortName(name: string): string {
	return name.trim().replace(/^(the|a|an)\s+/i, '').toLocaleLowerCase();
}

const time = (s?: string) => (s ? Date.parse(s) || 0 : 0);

/** have and total: aired episodes in the Library, or tracks. */
export function progressOf(t: TitleView): { have: number; total: number } {
	if (t.kind === 'show' && t.show) return { have: t.show.in_library, total: Math.max(0, t.show.episodes - t.show.not_out_yet) };
	if (t.kind === 'artist' && t.artist) return { have: t.artist.in_library, total: Math.max(0, t.artist.tracks - t.artist.not_out_yet) };
	const have = t.slots.some((s) => s.state === 'in_library' || s.state === 'better_wanted') ? 1 : 0;
	return { have, total: 1 };
}

export function releaseDate(t: TitleView): string | undefined {
	return t.digital_date ?? t.physical_date ?? t.cinema_date;
}

function nextDate(t: TitleView): string | undefined {
	return t.kind === 'show' ? t.show?.next_air_date : t.kind === 'artist' ? t.artist?.next_release : undefined;
}

/**
 * sortValue is what a sort compares. Missing dates and numbers sort last in
 * both directions (see sortTitles), so "Next episode" keeps shows with no
 * date at the bottom.
 */
function sortValue(t: TitleView, key: SortKey): number | string | null {
	switch (key) {
		case 'status': return libraryState(t)?.rank ?? null;
		case 'name': return sortName(t.name);
		case 'year': return t.year ?? null;
		case 'added': return time(t.added_at) || null;
		case 'size': return t.size_bytes ?? 0;
		case 'next': return time(nextDate(t)) || null;
		case 'last': return time(t.show?.last_air_date) || null;
		case 'release': return time(releaseDate(t)) || null;
		case 'runtime': return t.runtime_minutes || null;
		case 'network': return t.show?.network?.toLocaleLowerCase() || null;
		case 'rules': return t.pick_rules_name?.toLocaleLowerCase() || null;
		case 'progress': {
			const p = progressOf(t);
			return p.total ? p.have / p.total : null;
		}
	}
}

/** sortTitles sorts a copy; ties fall back to the name, and blanks sit last either way. */
export function sortTitles(ts: TitleView[], key: SortKey, dir: SortDir = 'asc'): TitleView[] {
	const sign = dir === 'asc' ? 1 : -1;
	return ts
		.map((t) => ({ t, v: sortValue(t, key), n: sortName(t.name) }))
		.sort((a, b) => {
			if (a.v === null && b.v !== null) return 1;
			if (b.v === null && a.v !== null) return -1;
			if (a.v !== null && b.v !== null && a.v !== b.v) return (a.v < b.v ? -1 : 1) * sign;
			return a.n < b.n ? -1 : a.n > b.n ? 1 : a.t.id - b.t.id;
		})
		.map((x) => x.t);
}

/** The sort each key starts with when picked: newest first for dates and sizes, soonest first for what is coming. */
export function startDir(key: SortKey): SortDir {
	return key === 'status' || key === 'name' || key === 'next' || key === 'network' || key === 'rules' || key === 'progress' ? 'asc' : 'desc';
}

/** defaultSort is each library's starting sort: newest added for movies and music, soonest next episode for TV. */
export function defaultSort(kind: LibKind | 'all'): { key: SortKey; dir: SortDir } {
	return kind === 'show' ? { key: 'next', dir: 'asc' } : kind === 'all' ? { key: 'name', dir: 'asc' } : { key: 'added', dir: 'desc' };
}

// ── Filters ─────────────────────────────────────────────────────────────

/** Kept is the "Keep up to date" filter: on, off or either. */
export type Kept = 'any' | 'on' | 'off';

export interface FilterState {
	status: Filter;
	kept: Kept;
	/** 'anyone', 'me' or a person's id. */
	asker: string;
	/** A movie series id, or 0 for every title. */
	series: number;
	/** A video codec (h264, hevc…) movies' main files must have, or '' for any. */
	codec: string;
	/** Only titles with subtitles still wanted. */
	subs: boolean;
}

export const noFilter: FilterState = { status: 'all', kept: 'any', asker: 'anyone', series: 0, codec: '', subs: false };

export interface FilterContext {
	askers: (t: TitleView) => number[];
	me?: number;
	seriesOf: (t: TitleView) => number | undefined;
	/** Whether the title has subtitles still wanted. */
	subsMissing?: (t: TitleView) => boolean;
}

export function passes(t: TitleView, f: FilterState, ctx: FilterContext, skip?: keyof FilterState): boolean {
	if (skip !== 'status' && !matches(t, f.status)) return false;
	if (skip !== 'kept' && f.kept !== 'any' && t.keep_up_to_date !== (f.kept === 'on')) return false;
	if (skip !== 'asker' && f.asker !== 'anyone') {
		const id = f.asker === 'me' ? ctx.me : Number(f.asker);
		if (id === undefined || !ctx.askers(t).includes(id)) return false;
	}
	if (skip !== 'series' && f.series && ctx.seriesOf(t) !== f.series) return false;
	if (skip !== 'codec' && f.codec && t.copy?.codec !== f.codec) return false;
	if (skip !== 'subs' && f.subs && !ctx.subsMissing?.(t)) return false;
	return true;
}

export interface FilterCounts {
	status: Record<Filter, number>;
	kept: Record<Kept, number>;
	askers: Map<number, number>;
	series: Map<number, number>;
	codecs: Map<string, number>;
	subs: number;
}

/**
 * filterCounts says how many titles each Filter menu choice would show, given
 * the other choices already made (so the counts never promise rows that the
 * other filters then hide).
 */
export function filterCounts(ts: TitleView[], f: FilterState, ctx: FilterContext): FilterCounts {
	const out: FilterCounts = {
		status: { all: 0, missing: 0, not_out_yet: 0, better: 0, have: 0 },
		kept: { any: 0, on: 0, off: 0 },
		askers: new Map(),
		series: new Map(),
		codecs: new Map(),
		subs: 0
	};
	const statuses: Filter[] = ['all', 'missing', 'not_out_yet', 'better', 'have'];
	for (const t of ts) {
		if (passes(t, f, ctx, 'status')) for (const s of statuses) if (matches(t, s)) out.status[s]++;
		if (passes(t, f, ctx, 'kept')) {
			out.kept.any++;
			out.kept[t.keep_up_to_date ? 'on' : 'off']++;
		}
		if (passes(t, f, ctx, 'asker')) for (const id of new Set(ctx.askers(t))) out.askers.set(id, (out.askers.get(id) ?? 0) + 1);
		if (passes(t, f, ctx, 'series')) {
			const s = ctx.seriesOf(t);
			if (s) out.series.set(s, (out.series.get(s) ?? 0) + 1);
		}
		if (t.copy && passes(t, f, ctx, 'codec')) out.codecs.set(t.copy.codec, (out.codecs.get(t.copy.codec) ?? 0) + 1);
		if (passes(t, f, ctx, 'subs') && ctx.subsMissing?.(t)) out.subs++;
	}
	return out;
}

export function activeCount(f: FilterState): number {
	return Number(f.status !== 'all') + Number(f.kept !== 'any') + Number(f.asker !== 'anyone') + Number(!!f.series) + Number(!!f.codec) + Number(!!f.subs);
}

/** Saved filters: a name and the choices, kept per member and library. */
export interface SavedFilter {
	name: string;
	filter: FilterState;
}

export function savedKey(member: number | undefined, kind: string): string {
	return `holyarrs.library-filters.${member ?? 'me'}.${kind}`;
}

export function loadSaved(member: number | undefined, kind: string, store: Pick<Storage, 'getItem'> | undefined = globalThis.localStorage): SavedFilter[] {
	try {
		const v = JSON.parse(store?.getItem(savedKey(member, kind)) ?? '[]');
		return Array.isArray(v) ? v.filter((s) => s && typeof s.name === 'string' && s.filter).map((s) => ({ name: s.name.slice(0, 60), filter: { ...noFilter, ...s.filter } })) : [];
	} catch {
		return [];
	}
}

export function saveSaved(member: number | undefined, kind: string, list: SavedFilter[], store: Pick<Storage, 'setItem'> | undefined = globalThis.localStorage) {
	try {
		store?.setItem(savedKey(member, kind), JSON.stringify(list.slice(0, 20)));
	} catch {
		// Not remembered in a private window.
	}
}

/**
 * tableMinWidth is the narrowest a table row may get: the smallest width of
 * every column (px, or a minmax's floor) plus the gaps and the end padding.
 * Every row uses it, so all rows share one width and the columns line up.
 */
export function tableMinWidth(widths: string[], gap = 16, pad = 20): number {
	const sum = widths.reduce((n, w) => n + (Number(/(\d+(?:\.\d+)?)px/.exec(w)?.[1]) || 0), 0);
	return Math.ceil(sum + gap * Math.max(0, widths.length - 1) + pad);
}
