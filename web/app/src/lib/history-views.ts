// History designs (Downloads > History, ?view=1…5): how much an Event
// matters, the verb and icon it is shown with, and the ways the designs fold
// a page of Events (by day, by title, routine bursts) so the few Events that
// matter are not lost among thousands of scans.
import type { IconName } from './Icon.svelte';
import { groupDownloads, historyQuery, kindGroups, type HistoryFilter, type HistoryRow } from './history.ts';
import type { EventRecord, TitleView } from './types.ts';

/** How much an Event matters: a result to look at, a change, or routine upkeep. */
export type Tier = 'important' | 'changes' | 'routine';

export const tiers: { key: Tier; label: string; hint: string }[] = [
	{ key: 'important', label: 'Important', hint: 'Imported, failed, deleted, needs a decision, health' },
	{ key: 'changes', label: 'Changes', hint: 'Upgraded, renamed, subtitles, added, asks, downloads' },
	{ key: 'routine', label: 'Routine', hint: 'Scans, searches and refreshed details' }
];

const groupTier: Record<string, Tier> = {
	import: 'important', fail: 'important', delete: 'important', decide: 'important', health: 'important',
	add: 'changes', hold: 'changes', choose: 'changes', grab: 'changes', download: 'changes', rename: 'changes',
	subtitles: 'changes', wishlist: 'changes', links: 'changes',
	search: 'routine', scan: 'routine'
};

/** Kinds no filter group names: a tier still finds them. */
const looseKinds: Record<string, Tier> = {
	albums_changed: 'routine', album_changed: 'routine',
	gate_opened: 'changes', season_changed: 'changes', title_edited: 'changes', secrets_imported: 'changes'
};

/** An upgrade is filed under Imported, but it is a change, not a result to look at. */
const UPGRADE = 'file_replaced';

function groupOf(kind: string): string {
	for (const g of kindGroups) {
		for (const k of g.kinds) if (k.endsWith('*') ? kind.startsWith(k.slice(0, -1)) : k === kind) return g.key;
	}
	return '';
}

/** tierOf says how much an Event kind matters. */
export function tierOf(kind: string): Tier {
	if (kind === UPGRADE) return 'changes';
	return looseKinds[kind] ?? groupTier[groupOf(kind)] ?? 'changes';
}

/** groupsOfTier lists the filter groups (kindGroups keys) shown under a tier. */
export function groupsOfTier(tier: Tier): string[] {
	return kindGroups.filter((g) => groupTier[g.key] === tier || (tier === 'changes' && g.key === 'import')).map((g) => g.key);
}

/**
 * tierKinds is the kind list a tier asks the server for. With groups picked,
 * only their kinds that belong to the tier.
 */
export function tierKinds(tier: Tier, groups: string[] = []): string[] {
	const picked = groups.length ? kindGroups.filter((g) => groups.includes(g.key)) : kindGroups;
	const out = picked.flatMap((g) => g.kinds).filter((k) => tierOf(k.replace(/\*$/, 'x')) === tier);
	if (!groups.length) for (const [k, t] of Object.entries(looseKinds)) if (t === tier) out.push(k);
	return out;
}

/** routineKinds is every kind of upkeep, for the "hide routine" switches. */
export const routineKinds = tierKinds('routine');
/** notRoutineKinds is every other kind (used to leave routine out). */
export const notRoutineKinds = [...tierKinds('important'), ...tierKinds('changes')];

/**
 * eventsQuery is the /events query for the page's filters, with the kinds
 * replaced when `kinds` is given (an empty list matches nothing, so callers
 * skip the call).
 */
export function eventsQuery(f: HistoryFilter, opts: { kinds?: string[]; limit?: number; offset?: number } = {}): string {
	const q = new URLSearchParams(historyQuery({ ...f, groups: opts.kinds ? [] : f.groups, limit: opts.limit ?? f.limit, offset: opts.offset ?? f.offset }));
	if (opts.kinds) q.set('kind', opts.kinds.join(','));
	return q.toString();
}

/** pickedKinds is the kinds of the picked groups, limited to a set (for "hide routine"). */
export function pickedKinds(groups: string[], within?: string[]): string[] | undefined {
	if (!groups.length) return within;
	const kinds = kindGroups.filter((g) => groups.includes(g.key)).flatMap((g) => g.kinds);
	if (!within) return kinds;
	const keep = new Set(within);
	return kinds.filter((k) => keep.has(k));
}

/** A verb, icon and tone an Event is shown with. */
export interface Verb {
	word: string;
	icon: IconName;
	tone: 'ok' | 'fail' | 'wait' | 'info' | 'idle';
}

/** verbOf names what happened in one word or two ("Imported", "Failed"). */
export function verbOf(kind: string): Verb {
	switch (kind) {
		case 'import_done': return { word: 'Imported', icon: 'check', tone: 'ok' };
		case 'import_partly_done': return { word: 'Partly imported', icon: 'check', tone: 'wait' };
		case UPGRADE: return { word: 'Upgraded', icon: 'sparkle', tone: 'info' };
		case 'free_space_low': return { word: 'Low on space', icon: 'alert', tone: 'fail' };
		case 'needs_decision': return { word: 'Needs a decision', icon: 'alert', tone: 'wait' };
		case 'stopped_looking': return { word: 'Stopped looking', icon: 'eyeOff', tone: 'idle' };
		case 'health_recovered': return { word: 'Fixed', icon: 'check', tone: 'ok' };
		case 'ask_made': return { word: 'Asked', icon: 'wish', tone: 'info' };
		case 'ask_approved': return { word: 'Approved', icon: 'check', tone: 'ok' };
		case 'ask_turned_down': return { word: 'Turned down', icon: 'declined', tone: 'idle' };
		case 'download_finished': return { word: 'Downloaded', icon: 'download', tone: 'ok' };
		case 'download_stalled': return { word: 'Stalled', icon: 'waiting', tone: 'wait' };
		case 'download_paused': return { word: 'Paused', icon: 'pause', tone: 'idle' };
		case 'download_queued': return { word: 'Queued', icon: 'waiting', tone: 'idle' };
		case 'file_missing': return { word: 'File missing', icon: 'alert', tone: 'fail' };
		case 'file_back': return { word: 'File back', icon: 'check', tone: 'ok' };
		case 'title_refreshed': return { word: 'Refreshed', icon: 'refresh', tone: 'idle' };
		case 'albums_changed':
		case 'album_changed': return { word: 'Albums changed', icon: 'music', tone: 'idle' };
		case 'gate_opened': return { word: 'Out now', icon: 'calendarDay', tone: 'info' };
		case 'title_edited':
		case 'season_changed': return { word: 'Changed', icon: 'settings', tone: 'idle' };
		case 'secrets_imported': return { word: 'Settings imported', icon: 'key', tone: 'idle' };
		case 'adopted': return { word: 'Adopted', icon: 'owned', tone: 'ok' };
	}
	switch (groupOf(kind)) {
		case 'add': return { word: 'Added', icon: 'plus', tone: 'ok' };
		case 'search': return { word: 'Searched', icon: 'search', tone: kind === 'search_failed' ? 'fail' : 'idle' };
		case 'hold': return { word: 'Held', icon: 'waiting', tone: 'wait' };
		case 'choose': return kind === 'release_turned_down' ? { word: 'Turned down', icon: 'declined', tone: 'idle' } : { word: 'Chosen', icon: 'star', tone: 'info' };
		case 'grab': return { word: 'Sent', icon: 'download', tone: 'info' };
		case 'download': return { word: 'Downloading', icon: 'download', tone: 'info' };
		case 'fail': return { word: 'Failed', icon: 'x', tone: 'fail' };
		case 'import': return { word: 'Imported', icon: 'check', tone: 'ok' };
		case 'rename': return { word: 'Renamed', icon: 'copy', tone: 'idle' };
		case 'delete': return { word: 'Deleted', icon: 'trash', tone: 'idle' };
		case 'subtitles': return { word: 'Subtitles', icon: 'subtitle', tone: /failed|none/.test(kind) ? 'wait' : 'ok' };
		case 'scan': return { word: 'Scanned', icon: 'refresh', tone: 'idle' };
		case 'decide': return { word: 'Needs a decision', icon: 'alert', tone: 'wait' };
		case 'health': return { word: 'Health', icon: 'alert', tone: 'fail' };
		case 'wishlist': return { word: 'Wishlist', icon: 'wish', tone: 'info' };
		case 'links': return { word: kind === 'alert_failed' ? 'Alert failed' : 'Media server', icon: 'bell', tone: /failed|unreachable/.test(kind) ? 'fail' : 'idle' };
	}
	return { word: 'Other', icon: 'info', tone: 'idle' };
}

/** headVerb is the verb of a History row: a folded download says how far it got. */
export function headVerb(row: HistoryRow): Verb {
	if (!row.steps.length) return verbOf(row.head.Kind);
	const failed = row.steps.find((s) => s.state === 'failed');
	if (failed) return { word: failed.key === 'imported' ? 'Import failed' : 'Download failed', icon: 'x', tone: 'fail' };
	if (row.events.some((e) => e.Kind === UPGRADE)) return verbOf(UPGRADE);
	const imported = row.steps.find((s) => s.key === 'imported');
	if (imported?.state === 'done') return verbOf('import_done');
	return verbOf(row.head.Kind);
}

/** rowTier is how much a row matters (a folded download by its worst or best step). */
export function rowTier(row: HistoryRow): Tier {
	let best: Tier = 'routine';
	for (const e of row.events) {
		const t = tierOf(e.Kind);
		if (t === 'important') return t;
		if (t === 'changes') best = t;
	}
	return best;
}

/** byWhom says who or what made an Event happen. */
export function byWhom(e: Pick<EventRecord, 'Actor' | 'Data'>): string {
	const d = e.Data ?? {};
	const name = (v: unknown) => (typeof v === 'string' ? v : Array.isArray(v) && typeof v[0] === 'string' ? v[0] : '');
	switch (e.Actor?.kind) {
		case 'user': return name(d.approved_by) || name(d.turned_down_by) || name(d.asked_by) || 'A person';
		case 'job': return 'Background job';
		case 'system': return 'HolyArrs';
	}
	return 'HolyArrs';
}

/** dayKey is the local day of a time (yyyy-mm-dd). */
export function dayKey(at: string): string {
	const d = new Date(at);
	return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

/** dayLabel names a day: Today, Yesterday, or "Sat, Oct 25" (with the year when it is not this year). */
export function dayLabel(key: string, now = new Date()): string {
	const today = dayKey(now.toISOString());
	const y = new Date(now);
	y.setDate(y.getDate() - 1);
	if (key === today) return 'Today';
	if (key === dayKey(y.toISOString())) return 'Yesterday';
	const [yy, mm, dd] = key.split('-').map(Number);
	const d = new Date(yy, mm - 1, dd);
	return d.toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric', ...(yy !== now.getFullYear() ? { year: 'numeric' } : {}) });
}

/** clock is the local time of day ("4:27 AM"). */
export function clock(at: string): string {
	return new Date(at).toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit' });
}

const plural = (n: number, one: string, many = one + 's') => `${n} ${n === 1 ? one : many}`;

/** The source named at the end of a refresh ("… from MusicBrainz"). */
function refreshSource(summary: string): string {
	return /\bfrom ([A-Za-z.]+)\s*$/.exec(summary)?.[1] ?? '';
}

/**
 * burstLine says in one line what a run of Events did ("Refreshed 24
 * artists from MusicBrainz · 3 searches for Slow Horses").
 */
export function burstLine(events: EventRecord[], titles: Map<number, Pick<TitleView, 'kind'>>): string {
	const parts: string[] = [];
	const by = new Map<string, EventRecord[]>();
	for (const e of events) {
		const k = e.Kind === 'album_changed' ? 'albums_changed' : e.Kind;
		by.set(k, [...(by.get(k) ?? []), e]);
	}
	for (const [kind, list] of by) {
		const ids = new Set(list.map((e) => e.TitleID ?? -e.ID));
		const names = [...new Set(list.map((e) => e.title_name).filter((n): n is string => !!n))];
		const forNames = names.length === 1 ? ` for ${names[0]}` : names.length > 1 ? ` for ${names[0]} and ${plural(names.length - 1, 'more', 'more')}` : '';
		if (kind === 'title_refreshed') {
			const kinds = new Set(list.map((e) => titles.get(e.TitleID ?? 0)?.kind ?? 'title'));
			const k = kinds.size === 1 ? [...kinds][0] : 'title';
			const word = k === 'artist' ? 'artist' : k === 'movie' ? 'movie' : k === 'show' ? 'show' : 'title';
			const sources = new Set(list.map((e) => refreshSource(e.Summary)).filter(Boolean));
			const from = sources.size === 1 ? ` from ${[...sources][0]}` : '';
			parts.push(ids.size === 1 && names[0] ? `Refreshed ${names[0]}${list.length > 1 ? ` ${list.length} times` : ''}${from}` : `Refreshed ${plural(ids.size, word)}${from}`);
		} else if (kind === 'albums_changed') {
			parts.push(ids.size === 1 && names[0] ? `New albums for ${names[0]}` : `Album lists changed for ${plural(ids.size, 'artist')}`);
		} else if (kind === 'search_done' || kind === 'search_failed' || kind === 'feed_checked') {
			parts.push(kind === 'feed_checked' ? plural(list.length, 'feed check') : `${plural(list.length, 'search', 'searches')}${forNames}`);
		} else if (kind === 'folder_scanned') {
			parts.push(`Scanned ${plural(list.length, 'folder')}`);
		} else if (kind === 'library_scanned' || kind === 'library_rescanned') {
			parts.push(`Scanned the Library${list.length > 1 ? ` ${list.length} times` : ''}`);
		} else if (list.length === 1) {
			parts.push(`${verbOf(kind).word}: ${list[0].title_name ?? list[0].Summary}`);
		} else {
			parts.push(`${verbOf(kind).word} ${plural(ids.size > 1 ? ids.size : list.length, ids.size > 1 ? 'title' : 'time')}${ids.size > 1 ? '' : forNames}`);
		}
	}
	return parts.join(' · ');
}

/** One item of the feed: a day header, one row, or a folded run of rows. */
export type FeedItem =
	| { type: 'day'; key: string; label: string; count: number }
	| { type: 'row'; key: string; row: HistoryRow; tier: Tier }
	| { type: 'burst'; key: string; rows: HistoryRow[]; tier: Tier; line: string };

/** routineFamily is the line a routine Event folds into: refreshes, searches or scans. */
export function routineFamily(kind: string): string {
	if (kind === 'title_refreshed' || kind === 'albums_changed' || kind === 'album_changed' || kind === 'episodes_changed' || kind === 'details_source_changed') return 'refresh';
	if (groupOf(kind) === 'search') return 'search';
	return 'scan';
}

/** The kind a run of rows folds by: routine folds together, others only with the same kind. */
function burstKey(row: HistoryRow, tier: Tier): string {
	if (tier === 'routine') return 'routine';
	if (row.steps.length) return '';
	return row.head.Kind;
}

/**
 * feedItems lays a page of Events out as a feed: a header per day, routine
 * runs folded into one line (two or more), and runs of three or more of the
 * same other kind (a clean-up deleting 20 titles) folded too.
 */
export function feedItems(events: EventRecord[], titles: Map<number, Pick<TitleView, 'kind'>>, now = new Date()): FeedItem[] {
	const out: FeedItem[] = [];
	let day = '';
	let dayHead: Extract<FeedItem, { type: 'day' }> | null = null;
	let run: HistoryRow[] = [];
	let runKey = '';
	let runTier: Tier = 'routine';
	const flush = () => {
		if (!run.length) return;
		if (runKey === 'routine') {
			// Upkeep folds by family (refreshes, searches, scans), each its own line, newest family first.
			const families = new Map<string, HistoryRow[]>();
			for (const r of run) {
				const f = routineFamily(r.head.Kind);
				families.set(f, [...(families.get(f) ?? []), r]);
			}
			for (const rows of families.values()) {
				if (rows.length >= 2) out.push({ type: 'burst', key: `b${rows[0].key}`, rows, tier: 'routine', line: burstLine(rows.flatMap((r) => r.events), titles) });
				else out.push({ type: 'row', key: rows[0].key, row: rows[0], tier: 'routine' });
			}
		} else if (runKey && run.length >= 3) {
			out.push({ type: 'burst', key: `b${run[0].key}`, rows: run, tier: runTier, line: burstLine(run.flatMap((r) => r.events), titles) });
		} else {
			for (const r of run) out.push({ type: 'row', key: r.key, row: r, tier: rowTier(r) });
		}
		run = [];
		runKey = '';
	};
	for (const row of groupDownloads(events)) {
		const d = dayKey(row.head.At);
		if (d !== day) {
			flush();
			day = d;
			dayHead = { type: 'day', key: `day${d}`, label: dayLabel(d, now), count: 0 };
			out.push(dayHead);
		}
		if (dayHead) dayHead.count += row.events.length;
		const tier = rowTier(row);
		const key = burstKey(row, tier);
		if (key && key === runKey) {
			run.push(row);
			continue;
		}
		flush();
		run = [row];
		runKey = key;
		runTier = tier;
	}
	flush();
	return out;
}

/** One title's story: its Events oldest first, folded into steps. */
export interface Story {
	key: string;
	titleId: number | null;
	name: string;
	/** Newest Event time (cards are ordered by it). */
	latest: string;
	steps: StoryStep[];
	/** Every step is routine (refreshes, scans, searches). */
	quiet: boolean;
	events: EventRecord[];
}

export interface StoryStep {
	key: string;
	verb: Verb;
	tier: Tier;
	/** The first Event of the step (oldest). */
	first: EventRecord;
	/** The newest Event of the step. */
	last: EventRecord;
	count: number;
	events: EventRecord[];
}

/** The step an Event is, so repeats ("Searched" 23 times in a row) fold into one. */
function stepWord(kind: string): string {
	const g = groupOf(kind);
	if (g === 'scan' || kind === 'albums_changed' || kind === 'album_changed') return 'Refreshed';
	return verbOf(kind).word;
}

/**
 * stories groups a page of Events by title (Events with no title share one
 * story per summary-less group, "HolyArrs itself"), each told oldest first
 * with repeated steps folded. Stories come newest first.
 */
export function stories(events: EventRecord[]): Story[] {
	const by = new Map<string, Story>();
	for (const e of events) {
		const key = e.TitleID ? `t${e.TitleID}` : 'none';
		let s = by.get(key);
		if (!s) {
			s = { key, titleId: e.TitleID ?? null, name: e.title_name ?? 'HolyArrs itself', latest: e.At, steps: [], quiet: true, events: [] };
			by.set(key, s);
		}
		s.events.push(e);
		if (e.At > s.latest) s.latest = e.At;
	}
	const out = [...by.values()];
	for (const s of out) {
		const oldestFirst = [...s.events].sort((a, b) => (a.At === b.At ? a.ID - b.ID : a.At < b.At ? -1 : 1));
		for (const e of oldestFirst) {
			const word = stepWord(e.Kind);
			const prev = s.steps[s.steps.length - 1];
			if (prev && stepWord(prev.last.Kind) === word && !(s.titleId === null && prev.last.Kind !== e.Kind)) {
				prev.count++;
				prev.last = e;
				prev.events.push(e);
				if (tierOf(e.Kind) === 'important') prev.tier = 'important';
				continue;
			}
			const tier = tierOf(e.Kind);
			s.steps.push({ key: `s${e.ID}`, verb: word === 'Refreshed' ? verbOf('title_refreshed') : verbOf(e.Kind), tier, first: e, last: e, count: 1, events: [e] });
		}
		s.quiet = s.steps.every((st) => st.tier === 'routine');
	}
	return out.sort((a, b) => (a.latest < b.latest ? 1 : a.latest > b.latest ? -1 : 0));
}

/** The table's sortable columns. */
export type SortKey = 'at' | 'title' | 'what' | 'by';

/** sortRows orders a page of rows by a column (time keeps the server's order for ties). */
export function sortRows(rows: HistoryRow[], key: SortKey, dir: 1 | -1): HistoryRow[] {
	const val = (r: HistoryRow): string => {
		switch (key) {
			case 'at': return r.head.At;
			case 'title': return (r.head.title_name ?? '￿').toLowerCase();
			case 'what': return headVerb(r).word.toLowerCase();
			case 'by': return byWhom(r.head).toLowerCase();
		}
	};
	return rows
		.map((r, i) => ({ r, i, v: val(r) }))
		.sort((a, b) => (a.v < b.v ? -dir : a.v > b.v ? dir : a.i - b.i))
		.map((x) => x.r);
}

/** titleHref is the link to a title still in the Library (removed titles have none). */
export function titleHref(id: number | null | undefined, titles: Map<number, unknown>): string | undefined {
	return id && titles.has(id) ? `/library/${id}` : undefined;
}

/** artOf is the poster (or the Event's own artwork) for an Event's title. */
export function artOf(e: Pick<EventRecord, 'TitleID' | 'artwork'>, titles: Map<number, Pick<TitleView, 'artwork'>>): string | undefined {
	return e.artwork ?? titles.get(e.TitleID ?? 0)?.artwork?.[0];
}

/** Debounce for live updates: a refresh run writes dozens of Events a minute. */
export function settle(fn: () => void, ms = 1500): { poke: () => void; stop: () => void } {
	let t: ReturnType<typeof setTimeout> | undefined;
	return {
		poke: () => {
			clearTimeout(t);
			t = setTimeout(fn, ms);
		},
		stop: () => clearTimeout(t)
	};
}
