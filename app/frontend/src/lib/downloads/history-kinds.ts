// History (Downloads > History): how much an Event matters, its verb and icon, its day and clock, and the line a run of routine Events folds into, so the few Events that matter are not lost among thousands.
import type { IconName } from '#lib/ui/Icon.svelte';
import { historyQuery, kindGroups, type HistoryFilter, type HistoryRow } from './history.ts';
import type { EventRecord, TitleView } from '../core/types.ts';

/** A result to look at, a change, or routine upkeep. */
export type Tier = 'important' | 'changes' | 'routine';

export interface Verb {
	word: string;
	icon: IconName;
	tone: 'ok' | 'fail' | 'wait' | 'info' | 'idle';
}

const groupTier: Record<string, Tier> = {
	import: 'important', fail: 'important', delete: 'important', decide: 'important', health: 'important',
	add: 'changes', hold: 'changes', choose: 'changes', grab: 'changes', download: 'changes', rename: 'changes',
	subtitles: 'changes', wishlist: 'changes', links: 'changes', ai: 'changes',
	search: 'routine', scan: 'routine'
};

/** Kinds no filter group names; a tier still finds them. */
const looseKinds: Record<string, Tier> = {
	albums_changed: 'routine', album_changed: 'routine',
	gate_opened: 'changes', season_changed: 'changes', title_edited: 'changes', secrets_imported: 'changes'
};

/** An upgrade is filed under Imported but is a change, not a result to look at. */
const UPGRADE = 'file_replaced';

/** How much an Event kind matters. */
export function tierOf(kind: string): Tier {
	if (kind === UPGRADE) return 'changes';
	return looseKinds[kind] ?? groupTier[groupOf(kind)] ?? 'changes';
}

/** Builds the /events query for the page's filters. `kinds` replaces the kinds when given; an empty list matches nothing, so callers skip the call. */
export function eventsQuery(f: HistoryFilter, opts: { kinds?: string[]; limit?: number; offset?: number } = {}): string {
	const q = new URLSearchParams(historyQuery({ ...f, groups: opts.kinds ? [] : f.groups, limit: opts.limit ?? f.limit, offset: opts.offset ?? f.offset }));
	if (opts.kinds) q.set('kind', opts.kinds.join(','));
	return q.toString();
}

/** Names what happened in a word or two ("Imported", "Failed"). */
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
		case 'report_made': return { word: 'Reported', icon: 'alert', tone: 'wait' };
		case 'report_fixed': return { word: 'Fixed', icon: 'check', tone: 'ok' };
		case 'report_reopened': return { word: 'Reopened', icon: 'alert', tone: 'wait' };
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
		case 'wishlist': return { word: 'Requests', icon: 'wish', tone: 'info' };
		case 'ai': return kind === 'ai_action_failed' ? { word: 'Not done', icon: 'sparkle', tone: 'fail' } : { word: 'By AI', icon: 'sparkle', tone: 'info' };
		case 'links': return { word: kind === 'alert_failed' ? 'Alert failed' : 'Media server', icon: 'bell', tone: /failed|unreachable/.test(kind) ? 'fail' : 'idle' };
	}
	return { word: 'Other', icon: 'info', tone: 'idle' };
}

/** How much a row matters; a folded download counts by its worst or best step. */
export function rowTier(row: HistoryRow): Tier {
	let best: Tier = 'routine';
	for (const e of row.events) {
		const t = tierOf(e.Kind);
		if (t === 'important') return t;
		if (t === 'changes') best = t;
	}
	return best;
}

/** Who or what made an Event happen. */
export function byWhom(e: Pick<EventRecord, 'Actor' | 'Data' | 'actor_name'>): string {
	const d = e.Data ?? {};
	const name = (v: unknown) => (typeof v === 'string' ? v : Array.isArray(v) && typeof v[0] === 'string' ? v[0] : '');
	switch (e.Actor?.kind) {
		case 'user': return name(d.approved_by) || name(d.turned_down_by) || name(d.asked_by) || e.actor_name || 'A person';
		// An AI app with a member's key: the app, then whose key.
		case 'ai': {
			const c = name(d.client) || 'An AI app';
			return c.charAt(0).toUpperCase() + c.slice(1) + (e.actor_name ? ` for ${e.actor_name}` : '');
		}
		case 'job': return 'Background job';
		case 'system': return 'HolyArrs';
	}
	return 'HolyArrs';
}

/** The local day of a time (yyyy-mm-dd). */
export function dayKey(at: string): string {
	const d = new Date(at);
	return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

/** Today, Yesterday, or "Sat, Oct 25" (with the year when it is not this year). */
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

/** The local time of day ("4:27 AM"). */
export function clock(at: string): string {
	return new Date(at).toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit' });
}

const plural = (n: number, one: string, many = one + 's') => `${n} ${n === 1 ? one : many}`;

/** One line for what a run of Events did ("Refreshed 24 artists from MusicBrainz · 3 searches for Slow Horses"). */
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

/** The link to a title still in the Library; removed titles have none. */
export function titleHref(id: number | null | undefined, titles: Map<number, unknown>): string | undefined {
	return id && titles.has(id) ? `/library/${id}` : undefined;
}

/** The poster, or the Event's own artwork, for an Event's title. */
export function artOf(e: Pick<EventRecord, 'TitleID' | 'artwork'>, titles: Map<number, Pick<TitleView, 'artwork'>>): string | undefined {
	return e.artwork ?? titles.get(e.TitleID ?? 0)?.artwork?.[0];
}

/** Debounce for live updates, since a refresh run writes dozens of Events a minute. */
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

function groupOf(kind: string): string {
	for (const g of kindGroups) {
		for (const k of g.kinds) if (k.endsWith('*') ? kind.startsWith(k.slice(0, -1)) : k === kind) return g.key;
	}
	return '';
}

/** The source named at the end of a refresh ("... from MusicBrainz"). */
function refreshSource(summary: string): string {
	return /\bfrom ([A-Za-z.]+)\s*$/.exec(summary)?.[1] ?? '';
}
