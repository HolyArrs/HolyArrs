// Library table: column layout, sorting, filtering and saved views.
import type { TitleView } from '../core/types.ts';
import { matches, type Filter } from './library.ts';
import { libraryState } from './library-state.ts';
import { shownScore } from '../core/rating.ts';
import { t as tr } from '../core/i18n/i18n.svelte.ts';

export type LibKind = 'movie' | 'show' | 'artist';

export type ColumnId =
	| 'status' | 'network' | 'rules' | 'next' | 'last' | 'seasons' | 'episodes' | 'added' | 'year' | 'size' | 'genres' | 'rating' | 'score' | 'path'
	| 'release' | 'runtime' | 'series' | 'albums' | 'tracks' | 'latest' | 'requested' | 'quality' | 'kept' | 'showstate' | 'activity' | 'codec' | 'audio';

export interface Column {
	id: ColumnId;
	label: string;
	// CSS grid track size.
	width: string;
	// Right-aligned values.
	numeric?: boolean;
	on: boolean;
}

export type ColumnLayout = { id: ColumnId; on: boolean }[];

export type SortKey = 'status' | 'name' | 'year' | 'added' | 'size' | 'next' | 'last' | 'progress' | 'release' | 'runtime' | 'network' | 'rules' | 'score';
export type SortDir = 'asc' | 'desc';

export type Kept = 'any' | 'on' | 'off';

export interface FilterState {
	status: Filter;
	kept: Kept;
	// "anyone", "me" or a member id.
	asker: string;
	// Collection id, 0 for any.
	series: number;
	// Codec name, empty for any.
	codec: string;
	// Only titles that still miss subtitles.
	subs: boolean;
}

export interface FilterContext {
	askers: (t: TitleView) => number[];
	me?: number;
	seriesOf: (t: TitleView) => number | undefined;
	// Set only when the subtitles part is on.
	subsMissing?: (t: TitleView) => boolean;
}

export interface FilterCounts {
	status: Record<Filter, number>;
	kept: Record<Kept, number>;
	askers: Map<number, number>;
	series: Map<number, number>;
	codecs: Map<string, number>;
	subs: number;
}

export interface SavedFilter {
	name: string;
	filter: FilterState;
}

// Each label reads its words when used, so it follows the language.
export const sortLabels = {} as Record<SortKey, string>;
for (const k of ['status', 'name', 'year', 'added', 'size', 'next', 'last', 'progress', 'release', 'runtime', 'network', 'rules', 'score'] as SortKey[]) {
	Object.defineProperty(sortLabels, k, { get: () => tr(`mediafiles.sort.${k}`), enumerable: true });
}

// Which sort a column header click uses; columns missing here are not sortable.
export const columnSort: Partial<Record<ColumnId, SortKey>> = {
	status: 'status',
	year: 'year', added: 'added', size: 'size', next: 'next', last: 'last', episodes: 'progress', tracks: 'progress',
	release: 'release', runtime: 'runtime', network: 'network', rules: 'rules', latest: 'next', score: 'score'
};

export const noFilter: FilterState = { status: 'all', kept: 'any', asker: 'anyone', series: 0, codec: '', subs: false };

// The label is the column's words in the current language at the time the columns are built.
const col = (id: ColumnId, width: string, on: boolean, numeric = false): Column => ({ id, label: tr(`mediafiles.col.${id}`), width, on, numeric });

const time = (s?: string) => (s ? Date.parse(s) || 0 : 0);

export function columnsFor(kind: LibKind): Column[] {
	if (kind === 'show')
		return [
			col('status', '200px', true),
			col('episodes', '150px', true),
			col('next', '118px', true, true),
			col('rules', '120px', true),
			col('size', '84px', true, true),
			col('added', '104px', true, true),
			col('network', '120px', false),
			col('seasons', '76px', false, true),
			col('last', '110px', false, true),
			col('year', '64px', false, true),
			col('genres', '160px', false),
			col('rating', '88px', false),
			col('score', '84px', false, true),
			col('requested', '130px', false),
			col('quality', '130px', false),
			col('kept', '72px', false),
			col('showstate', '100px', false),
			col('activity', '110px', false, true),
			col('path', 'minmax(160px, 1fr)', false)
		];
	if (kind === 'artist')
		return [
			col('status', '200px', true),
			col('tracks', '150px', true),
			col('latest', '118px', true, true),
			col('size', '84px', true, true),
			col('added', '104px', true, true),
			col('rules', '120px', true),
			col('albums', '84px', false, true),
			col('requested', '130px', false),
			col('kept', '72px', false),
			col('activity', '110px', false, true),
			col('path', 'minmax(160px, 1fr)', false)
		];
	return [
		col('status', '200px', true),
		col('release', '176px', true),
		col('rules', '120px', true),
		col('size', '84px', true, true),
		col('added', '104px', true, true),
		col('year', '64px', true, true),
		col('runtime', '84px', false, true),
		col('rating', '88px', false),
		col('score', '84px', false, true),
		col('series', '160px', false),
		col('genres', '160px', false),
		col('requested', '130px', false),
		col('quality', '130px', false),
		col('codec', '84px', false),
		col('audio', '150px', false),
		col('kept', '72px', false),
		col('activity', '110px', false, true),
		col('path', 'minmax(160px, 1fr)', false)
	];
}

// The columns that are on; while sorting by score its column shows right after the status, for this view only.
export function chosenColumns(cols: Column[], key: SortKey): Column[] {
	const on = cols.filter((c) => c.on && !(key === 'score' && c.id === 'score'));
	const score = key === 'score' && cols.find((c) => c.id === 'score');
	if (score) on.splice(on[0]?.id === 'status' ? 1 : 0, 0, { ...score, on: true });
	return on;
}

export function columnsKey(member: number | undefined, kind: LibKind): string {
	return `holyarrs.library-columns.${member ?? 'me'}.${kind}`;
}

// Applies a saved order and on/off state to the current columns; unknown ids are dropped and new columns slot in at their default place.
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
		// Storage can be blocked or full; the layout just will not be remembered.
	}
}

export function moveColumn(cols: Column[], from: number, to: number): Column[] {
	if (from === to || from < 0 || from >= cols.length) return cols;
	const next = [...cols];
	const [c] = next.splice(from, 1);
	next.splice(Math.max(0, Math.min(next.length, to)), 0, c);
	return next;
}

// The artist "next" sort is about releases, not episodes.
export function sortLabel(k: SortKey, kind: LibKind | 'all'): string {
	return k === 'next' && kind === 'artist' ? tr('mediafiles.sort.nextRelease') : sortLabels[k];
}

export function sortsFor(kind: LibKind | 'all'): SortKey[] {
	if (kind === 'show') return ['status', 'name', 'next', 'last', 'progress', 'added', 'year', 'score', 'size', 'network', 'rules'];
	if (kind === 'artist') return ['status', 'name', 'progress', 'added', 'size', 'next'];
	if (kind === 'movie') return ['status', 'name', 'year', 'score', 'release', 'added', 'size', 'runtime', 'rules'];
	return ['name', 'year', 'score', 'added', 'size'];
}

// Drops a leading article so "The Wire" sorts under W.
export function sortName(name: string): string {
	return name.trim().replace(/^(the|a|an)\s+/i, '').toLocaleLowerCase();
}

// Episodes and tracks not out yet do not count against completeness.
export function progressOf(t: TitleView): { have: number; total: number } {
	if (t.kind === 'show' && t.show) return { have: t.show.in_library, total: Math.max(0, t.show.episodes - t.show.not_out_yet) };
	if (t.kind === 'artist' && t.artist) return { have: t.artist.in_library, total: Math.max(0, t.artist.tracks - t.artist.not_out_yet) };
	const have = t.slots.some((s) => s.state === 'in_library' || s.state === 'better_wanted') ? 1 : 0;
	return { have, total: 1 };
}

export function releaseDate(t: TitleView): string | undefined {
	return t.digital_date ?? t.physical_date ?? t.cinema_date;
}

// Titles with no value always sink to the bottom, whichever way the sort goes; ties fall back to name, then id.
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

// Text sorts start A to Z; dates, sizes and counts start with the biggest or newest.
export function startDir(key: SortKey): SortDir {
	return key === 'status' || key === 'name' || key === 'next' || key === 'network' || key === 'rules' || key === 'progress' ? 'asc' : 'desc';
}

export function defaultSort(kind: LibKind | 'all'): { key: SortKey; dir: SortDir } {
	return kind === 'show' ? { key: 'next', dir: 'asc' } : kind === 'all' ? { key: 'name', dir: 'asc' } : { key: 'added', dir: 'desc' };
}

// "skip" leaves one filter out, so its own counts show what picking it would give.
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

export function savedKey(member: number | undefined, kind: string): string {
	return `holyarrs.library-filters.${member ?? 'me'}.${kind}`;
}

// Stored data may be old or hand-edited, so bad rows are dropped, missing filter fields get defaults and names are cut to 60.
export function loadSaved(member: number | undefined, kind: string, store: Pick<Storage, 'getItem'> | undefined = globalThis.localStorage): SavedFilter[] {
	try {
		const v = JSON.parse(store?.getItem(savedKey(member, kind)) ?? '[]');
		return Array.isArray(v) ? v.filter((s) => s && typeof s.name === 'string' && s.filter).map((s) => ({ name: s.name.slice(0, 60), filter: { ...noFilter, ...s.filter } })) : [];
	} catch {
		return [];
	}
}

// Keeps at most 20 saved views.
export function saveSaved(member: number | undefined, kind: string, list: SavedFilter[], store: Pick<Storage, 'setItem'> | undefined = globalThis.localStorage) {
	try {
		store?.setItem(savedKey(member, kind), JSON.stringify(list.slice(0, 20)));
	} catch {
		// Storage can be blocked or full; the view just will not be remembered.
	}
}

// Sums the fixed pixel tracks plus gaps and padding; flexible tracks add nothing. Defaults match the row gap and padding in CSS.
export function tableMinWidth(widths: string[], gap = 16, pad = 20): number {
	const sum = widths.reduce((n, w) => n + (Number(/(\d+(?:\.\d+)?)px/.exec(w)?.[1]) || 0), 0);
	return Math.ceil(sum + gap * Math.max(0, widths.length - 1) + pad);
}

function nextDate(t: TitleView): string | undefined {
	return t.kind === 'show' ? t.show?.next_air_date : t.kind === 'artist' ? t.artist?.next_release : undefined;
}

// Null means "no value", which sortTitles keeps at the bottom.
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
		case 'score': return shownScore(t.score, t.votes) ?? null;
		case 'progress': {
			const p = progressOf(t);
			return p.total ? p.have / p.total : null;
		}
	}
}
